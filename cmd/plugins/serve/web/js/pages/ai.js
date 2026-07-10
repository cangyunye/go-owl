export function renderAI(render, navigate, user, api, shell) {
  let system = { nodes: { total: 0, online: 0, offline: 0, warn: 0 } };
  let sessionId = null;
  let publicKeySpki = null;
  let chatMessages = [];

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

  shell.setPanelContent('');

  async function loadStats() {
    try {
      const res = await api.nodes({ page: 1, page_size: 200 });
      const nodes = res.data || [];
      system.nodes.total = nodes.length;
      system.nodes.online = nodes.filter(n => n.status === 'online').length;
      system.nodes.offline = nodes.filter(n => n.status === 'offline' || n.status === 'unknown').length;
      system.nodes.warn = nodes.filter(n => n.status === 'warn' || n.status === 'warning').length;
    } catch {}
    updateContext();
  }

  function updateContext() {
    const body = document.getElementById('agent-context-body');
    if (!body) return;
    body.innerHTML =
      '<div class="ctx-section-label">节点</div>' +
      '<div class="ctx-stat"><span class="lbl">总节点</span><span class="val">' + system.nodes.total + '</span></div>' +
      '<div class="ctx-stat"><span class="lbl">在线</span><span class="val online">' + system.nodes.online + '</span></div>' +
      '<div class="ctx-stat"><span class="lbl">离线</span><span class="val offline">' + system.nodes.offline + '</span></div>' +
      '<div class="ctx-stat"><span class="lbl">告警</span><span class="val warn">' + system.nodes.warn + '</span></div>';
  }

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

      let payload = { message: text };

      if (keyData && keyData.apiKey && sessionId && publicKeySpki) {
        const encryptedKey = await CryptoWallet.encryptApiKey(publicKeySpki, keyData.apiKey);
        payload.session_id = sessionId;
        payload.encrypted_api_key = encryptedKey;
      }

      const res = await api.aiChat(payload.message, payload.session_id, payload.encrypted_api_key);
      hideThinking();
      if (res && res.reply) {
        addMsg('assistant', esc(res.reply) + navChips(res.intent));
        chatMessages.push({ role: 'user', content: text });
        chatMessages.push({ role: 'assistant', content: res.reply });
        AIStorage.saveConversation({
          id: Date.now().toString(),
          messages: chatMessages,
          createdAt: new Date().toISOString()
        }).catch(() => {});
      }
    } catch (e) {
      hideThinking();
      addMsg('assistant', '抱歉，我现在无法响应。' + esc(e.message || '请稍后重试。'));
    }
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

  function handleSuggest(text) {
    sendMsg(text);
  }

  render(`
    <div class="agent-layout">
      <div class="agent-chat">
        <div class="suggest-chips">
          <span class="chip" onclick="handleSuggest('查看当前系统状态')">📊 系统概览</span>
          <span class="chip" onclick="handleSuggest('列出所有节点')">🖥️ 节点列表</span>
          <span class="chip" onclick="handleSuggest('在 web-01 上执行 df -h')">▶️ 执行命令</span>
          <span class="chip" onclick="handleSuggest('有哪些剧本可以运行？')">📜 剧本列表</span>
        </div>
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
        <div class="chat-input-bar">
          <input type="text" class="input" id="chat-input" placeholder="说点什么，例如「查看在线节点」「执行 uptime」…" aria-label="输入指令">
          <button class="btn btn-primary" id="send-btn" title="发送" aria-label="发送">
            <svg width="16" height="16" aria-hidden="true"><use href="#icon-send"/></svg>
          </button>
        </div>
      </div>
      <div class="agent-context" id="agent-context-panel">
        <div class="ctx-header">
          <svg width="14" height="14" aria-hidden="true"><use href="#icon-activity"/></svg>
          系统快照
        </div>
        <div class="ctx-body" id="agent-context-body">
          <div class="ctx-section-label">节点</div>
          <div class="ctx-stat"><span class="lbl">总节点</span><span class="val">0</span></div>
          <div class="ctx-stat"><span class="lbl">在线</span><span class="val online">0</span></div>
          <div class="ctx-stat"><span class="lbl">离线</span><span class="val offline">0</span></div>
          <div class="ctx-stat"><span class="lbl">告警</span><span class="val warn">0</span></div>
        </div>
      </div>
    </div>
  `, () => {
    loadStats();
    loadSessionKey();

    const input = document.getElementById('chat-input');
    const sendBtn = document.getElementById('send-btn');

    sendBtn.addEventListener('click', () => sendMsg(input.value));
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') sendMsg(input.value);
    });
  });

  // Make handleSuggest globally accessible for inline onclick
  window.handleSuggest = handleSuggest;
}
