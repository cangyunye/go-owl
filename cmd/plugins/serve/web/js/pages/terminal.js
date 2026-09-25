export function renderTerminal(render, navigate, user, api, nodeId, scope) {
  let ws = null;
  let term = null;
  let fitAddon = null;

  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  render(`
    <div class="terminal-page" style="flex:1;min-height:0;display:flex;flex-direction:column;margin:-20px -24px">
      <div class="terminal-header" style="display:flex;align-items:center;gap:10px;padding:8px 14px;border-bottom:1px solid var(--border);background:var(--surface)">
        <svg width="16" height="16" style="color:var(--accent)"><use href="#icon-terminal"/></svg>
        <strong>${esc(nodeId)}</strong>
        <span id="term-status" style="font-size:12px;color:var(--muted)">连接中…</span>
        <span style="flex:1"></span>
        <button class="btn btn-ghost btn-sm" id="term-reconnect">重连</button>
        <button class="btn btn-ghost btn-sm" id="term-close">关闭</button>
      </div>
      <div id="terminal-container" style="flex:1;min-height:0;background:#1a1b26"></div>
    </div>
  `, () => {
    const TerminalCtor = window.Terminal;
    const FitAddonCtor = window.FitAddon && window.FitAddon.FitAddon;
    const statusEl = document.getElementById('term-status');
    const setStatus = (txt, color) => { statusEl.textContent = txt; statusEl.style.color = color || 'var(--muted)'; };

    if (!TerminalCtor) {
      setStatus('终端组件加载失败', 'var(--danger)');
      return;
    }

    term = new TerminalCtor({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: { background: '#1a1b26' },
    });
    if (FitAddonCtor) {
      fitAddon = new FitAddonCtor();
      term.loadAddon(fitAddon);
    }

    const container = document.getElementById('terminal-container');
    term.open(container);
    if (fitAddon) fitAddon.fit();
    term.focus();

    async function connect() {
      let ticket;
      try {
        const res = await api.wsTicket();
        ticket = res.ticket;
      } catch (e) {
        setStatus('取连接票据失败', 'var(--danger)');
        return;
      }
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const url = `${protocol}//${window.location.host}/api/v1/session/terminal?node_id=${encodeURIComponent(nodeId)}&ticket=${encodeURIComponent(ticket)}&cols=${term.cols}&rows=${term.rows}`;
      setStatus('连接中…');
      ws = new WebSocket(url);
      ws.onopen = () => setStatus('已连接', 'var(--success)');
      ws.onmessage = (event) => {
        let msg;
        try { msg = JSON.parse(event.data); } catch { return; }
        if (msg.type === 'output') {
          term.write(msg.data);
        } else if (msg.type === 'exit') {
          term.write(`\r\n[会话结束，退出码 ${msg.code}]\r\n`);
          setStatus('已断开', 'var(--danger)');
        }
      };
      ws.onclose = () => setStatus('已断开', 'var(--danger)');
      ws.onerror = () => setStatus('连接错误', 'var(--danger)');
    }

    term.onData(data => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data }));
      }
    });

    // 失活标签的终端容器已从文档摘除：fit() 会量到 0 尺寸并误发 resize 帧，
    // 所以这条监听注册到作用域（自动随标签关闭回收，且失活期间被门控跳过）。
    const doResize = () => {
      if (!term || !term.element || !term.element.isConnected) return;
      if (fitAddon) fitAddon.fit();
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    };
    scope.resources.on(window, 'resize', doResize);

    connect();

    document.getElementById('term-close').addEventListener('click', () => navigate('/nodes'));
    document.getElementById('term-reconnect').addEventListener('click', () => {
      if (ws) ws.close();
      term.clear();
      connect();
    });

    // 切回本标签时补一次 fit：失活期间窗口可能已经变过尺寸
    scope.onResume(() => { if (term && term.element && term.element.isConnected) doResize(); });

    // 清理留给作用域：保活标签只在关闭/被替换时真正关闭 WS 与释放终端实例
    scope.resources.onDispose(() => {
      if (ws) ws.close();
      if (term) term.dispose();
    });
  });
}
