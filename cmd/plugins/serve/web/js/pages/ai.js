export function renderAI(render, navigate, user, api, shell) {
  let sessionId = null;
  let publicKeySpki = null;
  let chatMessages = [];
  let currentConvId = null;

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

  function addMsg(cls, html) {
    const m = document.getElementById('chat-messages');
    if (!m) return;
    const div = document.createElement('div');
    div.className = 'chat-msg ' + cls;
    if (cls === 'user') {
      div.innerHTML = '<div class="avatar">' + esc((user.display_name || user.username || 'U').charAt(0)) + '</div><div class="bubble">' + html + '</div>';
    } else {
      div.innerHTML = '<div class="avatar"><svg width="16" height="16"><use href="#icon-brain"/></svg></div><div class="bubble">' + html + '</div>';
    }
    m.appendChild(div);
    m.scrollTop = m.scrollHeight;
  }

  function showThinking() {
    const m = document.getElementById('chat-messages');
    if (!m) return;
    const el = document.createElement('div');
    el.className = 'chat-msg assistant';
    el.id = 'ai-thinking';
    el.innerHTML = '<div class="avatar"><svg width="16" height="16"><use href="#icon-brain"/></svg></div><div class="bubble"><div class="agent-thinking">分析中<div class="dots"><span></span><span></span><span></span></div></div></div>';
    m.appendChild(el);
    m.scrollTop = m.scrollHeight;
  }

  function hideThinking() {
    const el = document.getElementById('ai-thinking');
    if (el) el.remove();
  }

  async function sendMsg(text) {
    if (!text || !text.trim()) return;
    document.getElementById('chat-input').value = '';
    addMsg('user', esc(text));
    showThinking();

    try {
      const currentUser = user;
      const keyData = await AIStorage.loadApiKey(currentUser.id || currentUser.username);

      let payload = { message: text, provider: '', model: '', base_url: '', api_type: 'openai' };

      if (keyData && keyData.apiKey && sessionId && publicKeySpki) {
        const encryptedKey = await CryptoWallet.encryptApiKey(publicKeySpki, keyData.apiKey);
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
  }

  async function saveCurrentConv() {
    if (chatMessages.length === 0) return;
    const conv = {
      id: currentConvId || Date.now().toString(),
      messages: chatMessages,
      createdAt: new Date().toISOString()
    };
    if (!currentConvId) currentConvId = conv.id;
    await AIStorage.saveConversation(conv).catch(() => {});
    loadHistory();
  }

  async function loadHistory() {
    const list = document.getElementById('session-list');
    if (!list) return;
    try {
      const convs = await AIStorage.getConversations(50, 0);
      if (convs.length === 0) {
        list.innerHTML = '<div class="session-empty">暂无历史会话</div>';
        return;
      }
      list.innerHTML = convs.map(c => {
        const first = c.messages && c.messages.length > 0 ? c.messages[0].content : '(空)';
        const active = c.id === currentConvId ? ' session-item-active' : '';
        const date = new Date(c.createdAt);
        const label = date.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' }) + ' ' + date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
        return '<div class="session-item' + active + '" data-id="' + c.id + '">' +
          '<div class="session-title">' + esc(first.slice(0, 30)) + '</div>' +
          '<div class="session-meta">' + label + '</div>' +
          '</div>';
      }).join('');
      list.querySelectorAll('.session-item').forEach(el => {
        el.addEventListener('click', () => loadConversation(el.dataset.id, convs));
      });
    } catch {
      list.innerHTML = '<div class="session-empty">暂无历史会话</div>';
    }
  }

  async function loadConversation(id, convs) {
    const conv = convs.find(c => c.id === id);
    if (!conv) return;
    currentConvId = conv.id;
    chatMessages = conv.messages || [];
    const container = document.getElementById('chat-messages');
    if (!container) return;
    container.innerHTML = '';
    chatMessages.forEach(m => {
      const isUser = m.role === 'user';
      addMsg(isUser ? 'user' : 'assistant', isUser ? esc(m.content) : md(m.content));
    });
    // highlight active
    document.querySelectorAll('.session-item').forEach(el => el.classList.toggle('session-item-active', el.dataset.id === id));
  }

  function newConversation() {
    currentConvId = null;
    chatMessages = [];
    const container = document.getElementById('chat-messages');
    if (container) {
      container.innerHTML = '<div class="chat-msg assistant">' +
        '<div class="avatar"><svg width="16" height="16"><use href="#icon-brain"/></svg></div>' +
        '<div class="bubble">你好！我是 <strong>OWL Agent</strong> — 你的运维智能助手。<br><br>我可以帮你管理节点、执行命令、运行剧本和传输文件。试试输入指令或点击快捷建议。</div>' +
        '</div>';
    }
    document.querySelectorAll('.session-item').forEach(el => el.classList.remove('session-item-active'));
  }

  function navChips(intent) {
    const chips = {
      query_nodes: '/nodes',
      execute_command: '/exec',
      generate_playbook: '/playbooks',
      transfer_file: '/files',
    };
    const path = chips[intent];
    if (path) {
      return '<div class="agent-nav"><span class="nav-chip" onclick="window.location=\'' + path + '\'">前往操作 →</span></div>';
    }
    return '<div class="agent-nav">' +
      '<span class="nav-chip" onclick="window.location=\'/nodes\'">🖥️ 节点管理</span>' +
      '<span class="nav-chip" onclick="window.location=\'/exec\'">▶️ 命令执行</span>' +
      '<span class="nav-chip" onclick="window.location=\'/playbooks\'">📜 剧本管理</span>' +
      '</div>';
  }

  function fillPrompt(text) {
    const input = document.getElementById('chat-input');
    if (input) { input.value = text; input.focus(); }
  }

  render(`
    <div class="agent-layout">
      <div class="agent-chat">
        <div class="chat-messages" id="chat-messages">
          <div class="chat-msg assistant">
            <div class="avatar"><svg width="16" height="16"><use href="#icon-brain"/></svg></div>
            <div class="bubble">
              你好！我是 <strong>OWL Agent</strong> — 你的运维智能助手。<br><br>
              我可以帮你管理节点、执行命令、运行剧本和传输文件。
              试试输入指令或点击快捷建议。
            </div>
          </div>
        </div>
        <div class="prompt-chips" id="prompt-chips">
          <span class="pchip" data-prompt="查询所有在线节点的状态">📊 系统概览</span>
          <span class="pchip" data-prompt="列出所有节点">🖥️ 节点列表</span>
          <span class="pchip" data-prompt="在 web-01 上执行 df -h">▶️ 执行命令</span>
          <span class="pchip" data-prompt="有哪些剧本可以运行？">📜 剧本列表</span>
          <span class="pchip" data-prompt="传输 /etc/hosts 到 web-01">📁 传输文件</span>
        </div>
        <div class="chat-input-bar">
          <input type="text" class="input" id="chat-input" placeholder="输入指令，例如「查看在线节点」「执行 uptime」…" aria-label="输入指令">
          <button class="btn btn-primary" id="send-btn" title="发送" aria-label="发送">
            <svg width="16" height="16" aria-hidden="true"><use href="#icon-send"/></svg>
          </button>
        </div>
      </div>
      <div class="agent-context" id="agent-context-panel">
        <button class="session-new-btn" id="session-new-btn">+ 新增会话</button>
        <div class="session-list" id="session-list">
          <div class="session-empty">暂无历史会话</div>
        </div>
      </div>
    </div>
  `, () => {
    loadSessionKey();
    loadHistory();

    const input = document.getElementById('chat-input');
    const sendBtn = document.getElementById('send-btn');

    sendBtn.addEventListener('click', () => sendMsg(input.value));
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') sendMsg(input.value);
    });

    document.getElementById('session-new-btn').addEventListener('click', newConversation);

    document.querySelectorAll('.pchip').forEach(chip => {
      chip.addEventListener('click', () => fillPrompt(chip.dataset.prompt));
    });
  });
}
