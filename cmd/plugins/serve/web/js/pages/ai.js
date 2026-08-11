import { SlashMenu } from '../slash-menu.js';

export async function renderAI(render, navigate, user, api, shell) {
  let sessionId = null;
  let publicKeySpki = null;
  let chatMessages = [];
  let currentConvId = null;
  let isProcessing = false;

  // 会话归属用户命名空间：IndexedDB 历史与会话 id 均按当前登录用户隔离，
  // 避免同一浏览器下不同用户互相看到对方的 AI 对话。
  const userId = user?.id || user?.username || 'default';

  // ---- AI 会话权限 ----
  // 权限权威来源为后端 /api/v1/ai/permissions；拉取失败时按本地角色回退。
  // read_only 角色仅允许读工具；写工具（执行/传输/剧本运行/检查）仅 operator/admin。
  const AI_READ_TOOLS = ['query_nodes','query_database','node_status','node_groups','node_labels','list_playbooks','playbook_info','playbook_template_list','playbook_template_info','playbook_state_list','playbook_state_show','validate_playbook'];
  const AI_WRITE_TOOLS = ['execute_command','execute_script','generate_playbook','transfer_file','run_playbook','node_check'];
  let aiPerms = null;

  function canWriteRole(role) {
    return ['admin','operator'].includes(String(role || '').toLowerCase());
  }

  async function loadPermissions() {
    try {
      const p = await api.aiPermissions();
      if (p && Array.isArray(p.allowed_tools)) {
        aiPerms = p;
        return;
      }
    } catch (e) {
      console.error('Failed to load AI permissions, fallback to local role', e);
    }
    const canWrite = canWriteRole(user.role);
    aiPerms = {
      role: user.role || 'viewer',
      read_only: !canWrite,
      allowed_tools: canWrite ? AI_READ_TOOLS.concat(AI_WRITE_TOOLS) : AI_READ_TOOLS.slice(),
      blocked_tools: canWrite ? [] : AI_WRITE_TOOLS.slice(),
    };
  }

  function toolAllowed(tool) {
    if (!tool) return true;
    return !!(aiPerms && (aiPerms.allowed_tools || []).indexOf(tool) !== -1);
  }

  function readOnly() {
    return !!(aiPerms && aiPerms.read_only);
  }

  // ---- Slash command catalog ----
  // 任务类:展开为提示词模板;导航/系统类:直接执行动作。
  // tools 列表命中后端 allowed_tools 才展示；导航命令映射到对应页面所需写工具。
  const SLASH_COMMANDS = [
    { name: 'exec', category: 'task', tools: ['execute_command'], icon: '▶️', label: '执行命令', desc: '在指定节点上执行 shell 命令', template: '在 {nodes} 上执行命令 {command}', args: ['nodes', 'command'] },
    { name: 'check', category: 'task', tools: ['node_check'], icon: '🩺', label: '节点连通性检查', desc: '检查 SSH 连通性,支持所有/分组/指定节点,并列出不可达节点', template: '检查 {nodes} 的 SSH 连通性，找出不可达节点', args: ['nodes'] },
    { name: 'diagnose', category: 'task', tools: ['node_check'], icon: '🔍', label: '故障诊断', desc: '对目标节点或服务进行全栈故障诊断', template: '对 {target} 进行全栈故障诊断并给出修复建议', args: ['target'] },
    { name: 'query', category: 'task', tools: ['query_nodes'], icon: '📊', label: '节点查询', desc: '查询符合条件的节点信息', template: '查询 {condition} 的节点信息', args: ['condition'] },
    { name: 'playbook', category: 'task', tools: ['generate_playbook'], icon: '🛠️', label: '生成剧本', desc: '生成一个 Ansible playbook', template: '生成一个 playbook 实现 {requirement}', args: ['requirement'] },
    { name: 'transfer', category: 'task', tools: ['transfer_file'], icon: '📤', label: '传输文件', desc: '把文件传输到目标节点', template: '把 {source_file} 传输到 {nodes} 的 {dest_dir}', args: ['source_file', 'nodes', 'dest_dir'] },
    { name: 'script', category: 'task', tools: ['execute_script'], icon: '🧩', label: '执行脚本', desc: '在指定节点上运行脚本', template: '在 {nodes} 上运行脚本 {script}', args: ['nodes', 'script'] },

    { name: 'nodes', category: 'nav', icon: '🖥️', label: '节点管理', desc: '跳转到节点管理页', action: () => navigate('/nodes') },
    { name: 'exec-page', category: 'nav', tools: ['execute_command'], icon: '⚡', label: '命令执行页', desc: '跳转到命令执行页', action: () => navigate('/exec') },
    { name: 'playbooks', category: 'nav', tools: ['run_playbook'], icon: '📜', label: '剧本管理页', desc: '跳转到剧本管理页', action: () => navigate('/playbooks') },
    { name: 'files', category: 'nav', tools: ['transfer_file'], icon: '🗂️', label: '文件传输页', desc: '跳转到文件传输页', action: () => navigate('/files') },
    { name: 'new', category: 'nav', icon: '➕', label: '新建对话', desc: '开始一个新对话', action: () => newConversation() },
    { name: 'clear', category: 'nav', icon: '🗑️', label: '清空对话', desc: '删除当前对话并回到空态', action: () => clearConversation() },
    { name: 'help', category: 'nav', icon: 'ℹ️', label: '命令帮助', desc: '查看全部斜杠命令说明', action: (ta) => toggleHelp(ta) },
  ];

  function visibleCommands() {
    return SLASH_COMMANDS.filter((c) => (c.tools || []).every(toolAllowed));
  }


  async function loadSessionKey() {
    try {
      const data = await api.getSessionKey();
      sessionId = data.session_id;
      publicKeySpki = data.public_key_spki;
    } catch (e) {
      console.error('Failed to load session key', e);
    }
  }

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function md(text) {
    if (!text) return '';
    try { return marked.parse(text, { breaks: true, gfm: true }); }
    catch { return esc(text); }
  }

  shell.setPanelContent('');

  // ---- Message DOM ----
  function addMsg(cls, html) {
    const m = document.getElementById('ai-chat-messages');
    if (!m) return;
    // Ensure chat area is visible when adding messages
    m.style.display = 'block';
    const emptyState = document.getElementById('ai-empty-state');
    if (emptyState) emptyState.style.display = 'none';
    const div = document.createElement('div');
    div.className = 'ai-msg ' + cls;
    if (cls === 'user') {
      div.innerHTML = '<div class="ai-msg-avatar">' + esc((user.display_name || user.username || 'U').charAt(0)) + '</div><div class="ai-msg-body"><div class="ai-msg-bubble">' + html + '</div></div>';
    } else {
      div.innerHTML = '<div class="ai-msg-avatar"><svg width="16" height="16"><use href="#icon-brain"/></svg></div><div class="ai-msg-body"><div class="ai-msg-bubble">' + html + '</div></div>';
    }
    m.appendChild(div);
    m.scrollTop = m.scrollHeight;
    // hide suggestions once there are messages
    const sug = document.getElementById('ai-suggestions');
    if (sug) sug.style.display = 'none';
  }

  function showThinking() {
    const m = document.getElementById('ai-chat-messages');
    if (!m) return;
    const el = document.createElement('div');
    el.className = 'ai-msg ai';
    el.id = 'ai-typing-indicator';
    el.innerHTML = '<div class="ai-msg-avatar"><svg width="16" height="16"><use href="#icon-brain"/></svg></div><div class="ai-msg-body"><div class="ai-msg-bubble"><div class="ai-typing"><div class="ai-typing-dots"><span></span><span></span><span></span></div></div></div></div>';
    m.appendChild(el);
    m.scrollTop = m.scrollHeight;
  }

  function hideThinking() {
    const el = document.getElementById('ai-typing-indicator');
    if (el) el.remove();
  }

  // ---- Send message ----
  async function sendMsg(text) {
    if (!text || !text.trim() || isProcessing) return;
    text = text.trim();

    const input = document.getElementById('ai-chat-input');
    if (input) { input.value = ''; autoResize(); }

    addMsg('user', esc(text));
    isProcessing = true;
    updateSendBtn();
    showThinking();

    try {
      const currentUser = user;
      const keyData = await window.AIStorage.loadApiKey(currentUser.id || currentUser.username);

      let payload = { message: text, provider: '', model: '', base_url: '', api_type: 'openai' };

      if (keyData && keyData.apiKey && sessionId && publicKeySpki) {
        const encryptedKey = await window.CryptoWallet.encryptApiKey(publicKeySpki, keyData.apiKey);
        payload.session_id = sessionId;
        payload.encrypted_api_key = encryptedKey;
        payload.provider = keyData.provider || '';
        payload.model = keyData.model || '';
        payload.base_url = keyData.baseUrl || '';
        payload.api_type = keyData.apiFormat || 'openai';
      }

      const res = await api.aiChat(payload.message, payload.session_id, payload.encrypted_api_key, payload.provider, payload.model, payload.base_url, payload.api_type);
      hideThinking();
      if (res && res.reply) {
        addMsg('assistant', md(res.reply) + navChips(res.intent));
        chatMessages.push({ role: 'user', content: text });
        chatMessages.push({ role: 'assistant', content: res.reply });
        await saveCurrentConv();
      }
    } catch (e) {
      hideThinking();
      addMsg('assistant', '抱歉，我现在无法响应。' + esc(e.message || '请稍后重试。'));
    }
    isProcessing = false;
    updateSendBtn();
  }

  function updateSendBtn() {
    const btn = document.getElementById('ai-send-btn');
    const input = document.getElementById('ai-chat-input');
    if (btn) btn.disabled = isProcessing || !input || !input.value.trim();
  }

  function autoResize() {
    const ta = document.getElementById('ai-chat-input');
    if (!ta) return;
    ta.style.height = 'auto';
    ta.style.height = Math.min(ta.scrollHeight, 120) + 'px';
  }

  // ---- Conversation persistence ----
  async function saveCurrentConv() {
    if (chatMessages.length === 0) return;
    const conv = {
      id: currentConvId || (userId + '::' + Date.now()),
      userId: userId,
      messages: chatMessages,
      createdAt: new Date().toISOString()
    };
    if (!currentConvId) currentConvId = conv.id;
    await window.AIStorage.saveConversation(conv, userId).catch(() => {});
    loadHistory();
  }

  async function loadHistory() {
    const list = document.getElementById('ai-conv-list');
    if (!list) return;
    try {
      const convs = await window.AIStorage.getConversations(userId, 50, 0);
      if (convs.length === 0) {
        list.innerHTML = '<div class="ai-conv-empty">暂无历史会话</div>';
        return;
      }
      list.innerHTML = convs.map(c => {
        const first = c.messages && c.messages.length > 0 ? c.messages[0].content : '(空)';
        const active = c.id === currentConvId ? ' active' : '';
        const date = new Date(c.createdAt);
        const label = date.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' }) + ' ' + date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
        return '<div class="ai-conv-item' + active + '" data-id="' + c.id + '">' +
          '<div class="ai-conv-item-content">' +
          '<div class="ai-conv-item-title">' + esc(first.slice(0, 30)) + '</div>' +
          '<div class="ai-conv-item-time">' + label + '</div>' +
          '</div>' +
          '<button class="ai-conv-item-delete" data-id="' + c.id + '" title="删除会话" aria-label="删除会话">✕</button>' +
          '</div>';
      }).join('');
      list.querySelectorAll('.ai-conv-item').forEach(el => {
        el.addEventListener('click', (e) => {
          if (e.target.closest('.ai-conv-item-delete')) return;
          loadConversationById(el.dataset.id, convs);
        });
      });
      list.querySelectorAll('.ai-conv-item-delete').forEach(btn => {
        btn.addEventListener('click', (e) => {
          e.stopPropagation();
          deleteConversation(btn.dataset.id, convs);
        });
      });
    } catch {
      list.innerHTML = '<div class="ai-conv-empty">暂无历史会话</div>';
    }
  }

  async function loadConversationById(id, convs) {
    // If convs not provided, fetch fresh
    let list;
    if (!convs) {
      try { convs = await window.AIStorage.getConversations(userId, 50, 0); } catch { return; }
    }
    const conv = convs.find(c => c.id === id);
    if (!conv) return;
    currentConvId = conv.id;
    chatMessages = conv.messages || [];
    renderMessages();
    // highlight active
    document.querySelectorAll('.ai-conv-item').forEach(el => el.classList.toggle('active', el.dataset.id === id));
  }

  function renderMessages() {
    const container = document.getElementById('ai-chat-messages');
    const emptyState = document.getElementById('ai-empty-state');
    const suggestions = document.getElementById('ai-suggestions');
    if (!container) return;
    container.innerHTML = '';
    if (chatMessages.length === 0) {
      if (emptyState) emptyState.style.display = 'flex';
      if (suggestions) suggestions.style.display = 'flex';
      return;
    }
    if (container) container.style.display = 'block';
    if (emptyState) emptyState.style.display = 'none';
    if (suggestions) suggestions.style.display = 'none';
    chatMessages.forEach(m => {
      const isUser = m.role === 'user';
      addMsg(isUser ? 'user' : 'assistant', isUser ? esc(m.content) : md(m.content));
    });
  }

  function newConversation() {
    currentConvId = null;
    chatMessages = [];
    const chatArea = document.getElementById('ai-chat-messages');
    if (chatArea) chatArea.style.display = 'none';
    const emptyState = document.getElementById('ai-empty-state');
    if (emptyState) emptyState.style.display = 'flex';
    const sug = document.getElementById('ai-suggestions');
    if (sug) sug.style.display = 'flex';
    document.querySelectorAll('.ai-conv-item').forEach(el => el.classList.remove('active'));
  }

  async function clearConversation() {
    if (currentConvId) {
      try { await window.AIStorage.deleteConversation(currentConvId); } catch {}
    }
    const chatArea = document.getElementById('ai-chat-messages');
    if (chatArea) chatArea.innerHTML = '';
    newConversation();
    loadHistory();
  }

  function toggleHelp(textarea) {
    const main = textarea.closest('.ai-main');
    if (!main) return;
    const existing = document.getElementById('ai-help-overlay');
    if (existing) { existing.remove(); return; }

    const groups = {};
    visibleCommands().forEach((c) => { (groups[c.category] = groups[c.category] || []).push(c); });
    const groupTitle = { task: '任务', nav: '导航/系统' };

    const overlay = document.createElement('div');
    overlay.className = 'ai-help-overlay';
    overlay.id = 'ai-help-overlay';
    overlay.innerHTML =
      '<div class="ai-help-box">' +
      '<div class="ai-help-head"><span class="ai-help-title">斜杠命令</span>' +
      '<button class="ai-help-close" aria-label="关闭">✕</button></div>' +
      Object.keys(groups).map((k) =>
        '<div class="ai-help-group">' + (groupTitle[k] || k) + '</div>' +
        groups[k].map((c) =>
          '<div class="ai-help-item">' +
          '<span class="ai-help-name">/' + esc(c.name) + '</span>' +
          '<span class="ai-help-label">' + esc(c.label) + '</span>' +
          '<span class="ai-help-desc">' + esc(c.template || c.desc || '') + '</span>' +
          '</div>'
        ).join('')
      ).join('') +
      '<div class="ai-help-foot">输入 / 呼出补全,↑↓ 选择,Enter 确认,Esc 关闭</div>' +
      '</div>';

    main.appendChild(overlay);
    overlay.querySelector('.ai-help-close').addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
  }


  async function deleteConversation(id, convs) {
    try {
      await window.AIStorage.deleteConversation(id);
    } catch {}
    if (id === currentConvId) {
      newConversation();
    }
    loadHistory();
  }

  function navChips(intent) {
    const navTo = {
      query_nodes: '/nodes',
      execute_command: '/exec',
      generate_playbook: '/playbooks',
      transfer_file: '/files',
    };
    const path = navTo[intent];
    if (path && !readOnly()) {
      return '<div class="ai-msg-nav"><span class="ai-nav-chip" onclick="window.location=\'' + path + '\'">前往操作 →</span></div>';
    }
    const chips = ['<span class="ai-nav-chip" onclick="window.location=\'/nodes\'">🖥️ 节点管理</span>'];
    if (!readOnly()) {
      chips.push('<span class="ai-nav-chip" onclick="window.location=\'/exec\'">▶️ 命令执行</span>');
      chips.push('<span class="ai-nav-chip" onclick="window.location=\'/playbooks\'">📜 剧本管理</span>');
    }
    return '<div class="ai-msg-nav">' + chips.join('') + '</div>';
  }

  function fillPrompt(text) {
    const input = document.getElementById('ai-chat-input');
    if (input) { input.value = text; updateSendBtn(); autoResize(); input.focus(); }
  }

  // ---- Script list (话术列表) ----
  // tools 命中后端 allowed_tools 才展示，只读角色仅看到查询类话术。
  const SCRIPTS = [
    { icon: '🔍', text: '查询所有在线节点的状态和资源使用情况', tools: ['query_nodes'] },
    { icon: '⚡', text: '在 web-01 上执行 uptime 检查运行时长', tools: ['execute_command'] },
    { icon: '📊', text: '查看各节点 CPU 和内存使用率排行', tools: ['execute_command'] },
    { icon: '🛡️', text: '对线上服务进行全栈故障诊断', tools: ['node_check'] },
    { icon: '📋', text: '列出当前所有未处理的告警事件', tools: ['query_nodes'] },
    { icon: '📦', text: '下载近 24 小时所有服务的日志归档', tools: ['transfer_file'] },
    { icon: '🔧', text: '查询 nginx 网关服务的配置详情', tools: ['execute_command'] },
    { icon: '📈', text: '对比昨天和今天的流量变化趋势', tools: ['execute_command'] },
  ];

  function renderScriptList(container) {
    if (!container) return;
    const visible = SCRIPTS.filter((s) => (s.tools || []).every(toolAllowed));
    if (visible.length === 0) {
      container.innerHTML = '<div class="ai-conv-empty">当前角色暂无可用话术</div>';
      return;
    }
    container.innerHTML = visible.map(s =>
      '<div class="ai-script-item" data-prompt="' + esc(s.text) + '">' +
      '<span class="ai-script-item-icon">' + s.icon + '</span>' +
      '<span class="ai-script-item-text">' + esc(s.text) + '</span>' +
      '</div>'
    ).join('');
    container.querySelectorAll('.ai-script-item').forEach(el => {
      el.addEventListener('click', () => sendMessage(el.dataset.prompt));
    });
  }

  // ---- Capability buttons ----
  const CAPABILITIES = [
    { icon: '🔍', label: '故障诊断', prompt: '请对线上服务进行全面故障诊断，检查是否有异常', tools: ['node_check'] },
    { icon: '📊', label: '性能监控', prompt: '查看所有节点的性能监控指标，包括CPU、内存、磁盘和网络', tools: ['execute_command'] },
    { icon: '⚙️', label: '配置查询', prompt: '查询当前所有重要服务的配置概览', tools: ['execute_command'] },
    { icon: '🔔', label: '告警管理', prompt: '列出当前所有未处理的告警，按严重级别排序', tools: ['query_nodes'] },
    { icon: '📥', label: '下载日志', prompt: '下载近 24 小时所有服务的日志归档', tools: ['transfer_file'] },
  ];

  function renderCapabilities(container) {
    if (!container) return;
    const visible = CAPABILITIES.filter((c) => (c.tools || []).every(toolAllowed));
    container.innerHTML = visible.map(c =>
      '<button class="ai-cap-btn" data-prompt="' + esc(c.prompt) + '">' +
      '<span class="cap-icon">' + c.icon + '</span>' +
      c.label +
      '</button>'
    ).join('');
    container.querySelectorAll('.ai-cap-btn').forEach(btn => {
      btn.addEventListener('click', function() {
        fillPrompt(this.dataset.prompt);
        // brief active state
        document.querySelectorAll('.ai-cap-btn').forEach(b => b.classList.remove('active'));
        this.classList.add('active');
        setTimeout(() => this.classList.remove('active'), 2000);
      });
    });
  }

  // ---- Send message wrapper ----
  function sendMessage(text) {
    if (!text || !text.trim()) return;
    sendMsg(text.trim());
  }

  // ---- Provider selector ----
  const PROVIDER_LABELS = {
    anthropic: 'Anthropic', deepseek: 'DeepSeek', openai: 'OpenAI',
    qwen: '千问', volcengine: '火山引擎', minimax: 'MiniMax', mimo: '小米 MiMo', custom: '自定义'
  };
  const ALL_PROVIDERS = ['anthropic','deepseek','openai','qwen','volcengine','minimax','mimo','custom'];

  async function loadProviderSelector() {
    const sel = document.getElementById('ai-vendor-select');
    if (!sel) return;
    sel.innerHTML = '<option value="">— 选择供应商 —</option>';

    const userId = user?.id || user?.username || 'default';
    let foundOne = false;
    let activeProvider = '';

    for (const p of ALL_PROVIDERS) {
      let keyData = null;
      try {
        const raw = localStorage.getItem('owl_ai_key_' + p);
        if (raw) {
          const packet = JSON.parse(raw);
          const namespace = userId + '_' + p;
          keyData = await window.CryptoWallet.decryptLocal(packet, namespace);
        }
      } catch {}
      if (keyData && keyData.apiKey) {
        const label = PROVIDER_LABELS[p] || p;
        const model = keyData.model ? ' (' + keyData.model + ')' : '';
        sel.innerHTML += '<option value="' + p + '">' + label + model + '</option>';
        foundOne = true;
        if (!activeProvider) activeProvider = p;
      }
    }

    // Try to detect current active from legacy owl_ai_key
    try {
      const legacy = await window.AIStorage.loadApiKey(userId);
      if (legacy && legacy.apiKey) {
        for (const p of ALL_PROVIDERS) {
          const raw = localStorage.getItem('owl_ai_key_' + p);
          if (raw) {
            try {
              const packet = JSON.parse(raw);
              const namespace = userId + '_' + p;
              const kd = await window.CryptoWallet.decryptLocal(packet, namespace);
              if (kd && kd.apiKey === legacy.apiKey) {
                activeProvider = p;
                break;
              }
            } catch {}
          }
        }
      }
    } catch {}

    if (activeProvider) sel.value = activeProvider;

    if (!foundOne) {
      sel.innerHTML += '<option value="" disabled>— 请先在设置页配置供应商 —</option>';
    }
  }

  async function switchProvider(provider) {
    if (!provider) return;
    const userId = user?.id || user?.username || 'default';
    try {
      const raw = localStorage.getItem('owl_ai_key_' + provider);
      if (!raw) return;
      const packet = JSON.parse(raw);
      const namespace = userId + '_' + provider;
      const keyData = await window.CryptoWallet.decryptLocal(packet, namespace);
      if (keyData && keyData.apiKey) {
        const legacyPacket = await window.CryptoWallet.encryptLocal(
          { apiKey: keyData.apiKey, provider: keyData.provider, model: keyData.model, baseUrl: keyData.baseUrl, apiFormat: keyData.apiFormat || 'openai' },
          userId
        );
        localStorage.setItem('owl_ai_key', JSON.stringify(legacyPacket));
      }
    } catch {}
  }

  // ---- Suggestions (快捷短语) ----
  const SUGGESTIONS = [
    { icon: '📊', label: '系统概览', prompt: '查询所有在线节点的状态', tools: ['query_nodes'] },
    { icon: '🖥️', label: '节点列表', prompt: '列出所有节点', tools: ['query_nodes'] },
    { icon: '▶️', label: '执行命令', prompt: '在 web-01 上执行 df -h', tools: ['execute_command'] },
    { icon: '📜', label: '剧本列表', prompt: '有哪些剧本可以运行？', tools: ['list_playbooks'] },
    { icon: '📁', label: '传输文件', prompt: '传输 /etc/hosts 到 web-01', tools: ['transfer_file'] },
  ];

  function visibleSuggestions() {
    return SUGGESTIONS.filter((s) => (s.tools || []).every(toolAllowed));
  }

  // ---- Render ----
  await loadPermissions();

  render(`
    <div class="ai-layout" data-ai-theme="moonlight">
      <!-- Left Sidebar: Conversation list -->
      <div class="ai-sidebar-left">
        <div class="ai-sidebar-header">
          <span class="ai-sidebar-header-title">会话</span>
        </div>
        <button class="ai-new-conv-btn" id="ai-new-conv-btn">＋ 新建对话</button>
        <div class="ai-conv-list" id="ai-conv-list">
          <div class="ai-conv-empty">暂无历史会话</div>
        </div>
      </div>

      <!-- Main: Chat area -->
      <div class="ai-main">
        <!-- Header -->
        <div class="ai-header">
          <div class="ai-header-left">
            <div class="ai-header-icon"><svg width="18" height="18"><use href="#icon-brain"/></svg></div>
            <span class="ai-header-title">OPS AI</span>
            <span class="ai-header-badge">v2.1</span>
            <span class="ai-status-dot"></span>
            <span class="ai-status-text">已就绪</span>
          </div>
          <div class="ai-header-right">
          </div>
        </div>

        <!-- Empty state (shown initially) -->
        <div class="ai-empty-state" id="ai-empty-state">
          <div class="ai-empty-icon"><svg width="28" height="28"><use href="#icon-brain"/></svg></div>
          <div class="ai-empty-title">开始对话</div>
          <div class="ai-empty-desc">
            ${readOnly()
              ? '我是 <strong>OPS AI</strong> — 你的运维智能助手。<br>当前为 <strong>只读权限</strong>，可帮你查询节点、查看剧本与运行记录。<br>执行命令、脚本、传输文件、运行剧本等写操作需 <strong>operator</strong> 或 <strong>admin</strong> 角色。'
              : '我是 <strong>OPS AI</strong> — 你的运维智能助手。<br>可以帮你管理节点、执行命令、运行剧本和传输文件。<br>输入指令或点击下方快捷短语开始。'}
          </div>
        </div>

        <!-- Read-only permission banner -->
        ${readOnly() ? '<div class="ai-readonly-banner" id="ai-readonly-banner">⚠️ 当前角色 <strong>' + esc(aiPerms.role || 'viewer') + '</strong>：AI 会话仅支持查询与查看。执行命令 / 脚本、文件传输、剧本运行、节点检查等操作需 <strong>operator</strong> 或 <strong>admin</strong> 角色。</div>' : ''}

        <!-- Messages -->
        <div class="ai-chat-area" id="ai-chat-messages"></div>

        <!-- Suggestions -->
        <div class="ai-suggestions" id="ai-suggestions">
          ${visibleSuggestions().map((s) => '<span class="ai-suggest-btn" data-prompt="' + esc(s.prompt) + '">' + s.icon + ' ' + esc(s.label) + '</span>').join('')}
        </div>

        <!-- Toolbar -->
        <div class="ai-toolbar">
          <div class="ai-capabilities" id="ai-capabilities"></div>
          <div class="ai-vendor-group">
            <span class="ai-vendor-label">供应商</span>
            <select class="ai-vendor-select" id="ai-vendor-select">
              <option value="">— 选择供应商 —</option>
            </select>
          </div>
        </div>

        <!-- Input area -->
        <div class="ai-input-area">
          <textarea id="ai-chat-input" placeholder="输入指令,例如「查看在线节点」「执行 uptime」…输入 / 呼出命令补全" aria-label="输入指令" rows="1"></textarea>
          <button class="ai-send-btn" id="ai-send-btn" title="发送" aria-label="发送" disabled>
            <svg width="16" height="16" aria-hidden="true"><use href="#icon-send"/></svg>
          </button>
        </div>
      </div>

      <!-- Right Sidebar: Scripts -->
      <div class="ai-sidebar-right">
        <div class="ai-sidebar-right-header">
          <span class="ai-sidebar-right-title">话术列表</span>
        </div>
        <div class="ai-script-list" id="ai-script-list"></div>
      </div>
    </div>
  `, () => {
    // Hide shell panel and toggle for AI full-width layout
    const sidePanel = document.getElementById('sidePanel');
    const panelToggle = document.getElementById('panelToggle');
    const viewContainer = document.querySelector('.view-container');
    if (sidePanel) sidePanel.style.display = 'none';
    if (panelToggle) panelToggle.style.display = 'none';
    if (viewContainer) viewContainer.style.padding = '0';

    // Init
    loadSessionKey();
    loadHistory();

    // Restore messages if we have a current conversation
    const emptyState = document.getElementById('ai-empty-state');
    const chatArea = document.getElementById('ai-chat-messages');
    const suggestions = document.getElementById('ai-suggestions');

    // If we loaded history and found a current conversation, show messages
    if (currentConvId && chatMessages.length > 0) {
      if (emptyState) emptyState.style.display = 'none';
      if (chatArea) chatArea.style.display = 'block';
      if (suggestions) suggestions.style.display = 'none';
    } else {
      if (chatArea) chatArea.style.display = 'none';
      if (emptyState) emptyState.style.display = 'flex';
      if (suggestions) suggestions.style.display = 'flex';
    }

    // New conversation
    document.getElementById('ai-new-conv-btn').addEventListener('click', newConversation);

    // Send
    const input = document.getElementById('ai-chat-input');
    const sendBtn = document.getElementById('ai-send-btn');
    const slash = new SlashMenu(input, { commands: visibleCommands() });

    sendBtn.addEventListener('click', () => sendMsg(input.value));

    input.addEventListener('input', () => { updateSendBtn(); autoResize(); });

    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey && !e.defaultPrevented) {
        e.preventDefault();
        sendMsg(input.value);
      }
    });

    // Suggestions
    document.querySelectorAll('.ai-suggest-btn').forEach(chip => {
      chip.addEventListener('click', () => fillPrompt(chip.dataset.prompt));
    });

    // Capabilities
    renderCapabilities(document.getElementById('ai-capabilities'));

    // Scripts
    renderScriptList(document.getElementById('ai-script-list'));

    // Provider
    loadProviderSelector();
    document.getElementById('ai-vendor-select').addEventListener('change', (e) => {
      switchProvider(e.target.value);
    });

    updateSendBtn();

    return () => slash.destroy();
  });
}
