export function renderTaskDetail(render, navigate, user, api, taskId) {
  let wsCleanup = null;

  render(`
    <div class="app-header">
      <h1>OWL Console</h1>
      <div class="header-right">
        <a href="/" style="font-size:14px;color:var(--text-muted)">Nodes</a>
        <a href="/tasks" style="font-size:14px;color:var(--text)">Tasks</a>
        ${user.role === 'admin' || user.role === 'operator' ? '<a href="/playbooks" style="font-size:14px;color:var(--text-muted)">Playbooks</a>' : ''}
        ${user.role === 'admin' ? '<a href="/settings" style="font-size:14px;color:var(--text-muted)">Settings</a>' : ''}
        ${user.role === 'admin' ? '<a href="/users" style="font-size:14px;color:var(--text-muted)">Users &amp; Permissions</a>' : ''}
        <span>${esc(user.display_name || user.username)}</span>
        <span class="role-badge">${esc(user.role)}</span>
        <button class="logout-btn" id="logout-btn">Sign Out</button>
      </div>
    </div>
    <div class="app-content">
      <a href="/tasks" class="back-link">&larr; Back to Tasks</a>
      <div id="task-content" class="loading">Loading...</div>
    </div>
  `, () => {
    document.getElementById('logout-btn').addEventListener('click', () => { localStorage.removeItem('token'); localStorage.removeItem('user'); navigate('/login'); });
    load();
    wsCleanup = api.connectWebSocket((msg) => {
      if (msg.type === 'task_update' && msg.data && msg.data.id === taskId) {
        renderTask(msg.data);
      }
    });
    return () => { if (wsCleanup) wsCleanup.close(); };
  });

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  function statusDot(s) { return '<span class="status-dot '+s+'"></span>'; }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'s'; if (s<3600) return Math.floor(s/60)+'m'; return Math.floor(s/3600)+'h'; }

  function renderTask(t) {
    const container = document.getElementById('task-content');
    if (!container) return;
    const duration = t.started_at && t.completed_at
      ? Math.floor((new Date(t.completed_at) - new Date(t.started_at))/1000)+'s'
      : t.started_at ? timeAgo(t.started_at) : '-';
    container.innerHTML =
      '<div class="page-header">' +
        '<h2 style="font-family:\'SF Mono\',Monaco,monospace;font-size:16px">'+esc(t.id.slice(0,8))+'</h2>' +
        '<span class="status-badge status-'+(t.status === 'completed' ? 'online' : t.status === 'failed' || t.status === 'cancelled' ? 'offline' : 'unknown')+'">'+statusDot(t.status)+esc(t.status)+'</span>' +
      '</div>' +
      '<div class="card" style="margin-bottom:16px">' +
        '<div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:16px">' +
          '<div class="detail-field"><label>Node</label><div class="value" style="font-family:\'SF Mono\',Monaco,monospace">'+esc(t.node_id)+'</div></div>' +
          '<div class="detail-field"><label>Exit Code</label><div class="value" style="font-family:\'SF Mono\',Monaco,monospace">'+(t.exit_code !== null && t.exit_code !== undefined ? t.exit_code : '-')+'</div></div>' +
          '<div class="detail-field"><label>Duration</label><div class="value" style="font-family:\'SF Mono\',Monaco,monospace">'+duration+'</div></div>' +
        '</div>' +
        '<div class="detail-field" style="margin-top:12px"><label>Command</label><div class="value" style="font-family:\'SF Mono\',Monaco,monospace;font-size:13px;background:var(--bg);padding:8px 10px;border-radius:var(--radius);margin-top:4px;word-break:break-all">'+esc(t.command)+'</div></div>' +
      '</div>' +
      '<div class="card">' +
        '<div class="detail-field"><label>Output</label></div>' +
        '<div class="stream-output" id="stream-output">'+(t.output ? esc(t.output) : '<span style="color:var(--text-muted)">(awaiting output...)</span>')+'</div>' +
      '</div>' +
      ((t.status === 'queued' || t.status === 'running') && user.role === 'admin'
        ? '<button class="btn btn-danger" id="cancel-btn" style="margin-top:16px">Cancel Task</button>'
        : '');
    const cancelBtn = document.getElementById('cancel-btn');
    if (cancelBtn) {
      cancelBtn.addEventListener('click', async () => {
        try { await api.cancelTask(taskId); } catch (e) { alert(e.message); }
      });
    }
    const outputEl = document.getElementById('stream-output');
    if (outputEl && t.status === 'running') {
      outputEl.scrollTop = outputEl.scrollHeight;
    }
  }

  async function load() {
    try {
      const t = await api.task(taskId);
      renderTask(t);
    } catch (e) {
      const container = document.getElementById('task-content');
      if (container) container.innerHTML = `<p style="color:var(--danger)">${esc(e.message)}</p>`;
    }
  }

}
