export function renderDashboard(render, navigate, user, api, shell) {
  let stats = { total: 0, online: 0, offline: 0, warn: 0 };
  let recentTasks = [];

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'s'; if (s<3600) return Math.floor(s/60)+'m'; return Math.floor(s/3600)+'h'; }

  async function loadAll() {
    try {
      const [nodeRes, taskRes] = await Promise.all([
        api.nodes({ page: 1, page_size: 100 }),
        api.tasks({ page: 1, page_size: 5 })
      ]);
      const nodes = nodeRes.data || [];
      stats.total = nodes.length;
      stats.online = nodes.filter(n => n.status === 'online').length;
      stats.offline = nodes.filter(n => n.status === 'offline' || n.status === 'unknown').length;
      stats.warn = nodes.filter(n => n.status === 'warn' || n.status === 'warning').length;
      recentTasks = taskRes.data || [];
    } catch {}
    renderCards();
  }

  function statusDot(s) {
    if (s === 'completed') return 'var(--success)';
    if (s === 'failed' || s === 'cancelled') return 'var(--danger)';
    if (s === 'running') return 'var(--warn)';
    return 'var(--muted)';
  }

  function statusText(s) {
    const map = { completed: '成功', failed: '失败', cancelled: '已取消', running: '进行中', pending: '等待中' };
    return map[s] || s;
  }

  function renderCards() {
    const onlineRate = stats.total > 0 ? Math.round(stats.online / stats.total * 100) : 0;
    const successRate = 98;

    document.getElementById('stat-total').textContent = stats.total;
    document.getElementById('stat-rate').textContent = onlineRate + '%';
    document.getElementById('stat-online').textContent = stats.online;
    document.getElementById('stat-offline').textContent = stats.offline;

    renderDonut();
    renderTasks();
  }

  function renderDonut() {
    const total = stats.total || 1;
    const online = stats.online;
    const offline = stats.offline;
    const warn = stats.warn;
    const onlinePct = online / total;
    const offlinePct = offline / total;
    const warnPct = warn / total;
    const onlineLen = Math.round(onlinePct * 339);
    const offlineLen = Math.round(offlinePct * 339);
    const warnLen = Math.round(warnPct * 339);
    const remaining = 339 - onlineLen - offlineLen - warnLen;

    const donut = document.getElementById('donut-svg');
    if (!donut) return;
    let segments = '';
    if (onlineLen > 0) segments += `<circle cx="60" cy="60" r="54" fill="none" stroke="var(--success)" stroke-width="12" stroke-dasharray="${onlineLen} ${339 - onlineLen}" stroke-dashoffset="0" stroke-linecap="round"/>`;
    if (warnLen > 0) segments += `<circle cx="60" cy="60" r="54" fill="none" stroke="var(--warn)" stroke-width="12" stroke-dasharray="${warnLen} ${339 - warnLen}" stroke-dashoffset="${-onlineLen}" stroke-linecap="round"/>`;
    if (offlineLen > 0) segments += `<circle cx="60" cy="60" r="54" fill="none" stroke="var(--muted)" stroke-width="12" stroke-dasharray="${offlineLen} ${339 - offlineLen}" stroke-dashoffset="${-onlineLen - warnLen}" stroke-linecap="round"/>`;
    if (remaining > 0) segments += `<circle cx="60" cy="60" r="54" fill="none" stroke="var(--border)" stroke-width="12" stroke-dasharray="${remaining} ${339 - remaining}" stroke-dashoffset="${-onlineLen - warnLen - offlineLen}" stroke-linecap="round"/>`;

    donut.innerHTML = segments;
    document.getElementById('donut-total').textContent = stats.total;
    document.getElementById('legend-online').textContent = stats.online;
    document.getElementById('legend-offline').textContent = stats.offline;
    document.getElementById('legend-warn').textContent = stats.warn;
  }

  function renderTasks() {
    const list = document.getElementById('recent-tasks');
    if (!list) return;
    if (recentTasks.length === 0) {
      list.innerHTML = '<li class="task-item"><div class="task-info"><div class="task-name" style="color:var(--muted)">暂无最近任务</div></div></li>';
    } else {
      list.innerHTML = recentTasks.map(t => {
        const isRunning = t.status === 'running';
        return `<li class="task-item">
          ${isRunning ? '<span class="status-pulse" style="background:var(--warn)"></span>' : `<span class="status-icon" style="background:${statusDot(t.status)}"></span>`}
          <div class="task-info">
            <div class="task-name">${esc(t.command || t.name || '')}</div>
            <div class="task-meta">节点: ${esc(t.node_id)}</div>
          </div>
          <span class="task-time">${statusText(t.status)}</span>
        </li>`;
      }).join('');
    }
  }

  shell.setPanelContent(`
    <li class="panel-item active"><span class="dot" style="background:var(--accent)"></span>全部节点 <span class="count" id="panel-total">${stats.total}</span></li>
    <li class="panel-item"><span class="dot" style="background:var(--success)"></span>在线 <span class="count" id="panel-online">${stats.online}</span></li>
    <li class="panel-item"><span class="dot" style="background:var(--muted)"></span>离线 <span class="count" id="panel-offline">${stats.offline}</span></li>
    <li class="panel-item"><span class="dot" style="background:var(--warn)"></span>告警 <span class="count" id="panel-warn">${stats.warn}</span></li>
  `);

  render(`
    <div class="stats-grid">
      <div class="stat-card">
        <div class="label"><svg width="14" height="14" aria-hidden="true"><use href="#icon-nodes"/></svg> 总节点</div>
        <div class="value" id="stat-total">0</div>
      </div>
      <div class="stat-card">
        <div class="label"><span class="status-dot online" style="width:8px;height:8px"></span> 在线率</div>
        <div class="value" id="stat-rate">0%</div>
      </div>
      <div class="stat-card">
        <div class="label"><svg width="14" height="14" aria-hidden="true"><use href="#icon-play"/></svg> 在线节点</div>
        <div class="value" id="stat-online">0</div>
      </div>
      <div class="stat-card">
        <div class="label"><svg width="14" height="14" aria-hidden="true"><use href="#icon-alert-circle"/></svg> 离线节点</div>
        <div class="value" id="stat-offline">0</div>
      </div>
    </div>

    <div class="chart-row">
      <div class="card">
        <div class="card-header">
          <h3>节点分布</h3>
        </div>
        <div class="card-body">
          <div class="donut-wrapper">
            <div class="donut">
              <svg width="120" height="120" viewBox="0 0 120 120">
                <circle cx="60" cy="60" r="54" fill="none" stroke="var(--border)" stroke-width="12"/>
                <g id="donut-svg"></g>
              </svg>
              <div class="center-text" id="donut-total">0</div>
            </div>
            <div class="donut-legend">
              <div class="legend-item"><span class="swatch" style="background:var(--success)"></span> 在线 <span class="l-val" id="legend-online">0</span></div>
              <div class="legend-item"><span class="swatch" style="background:var(--muted)"></span> 离线 <span class="l-val" id="legend-offline">0</span></div>
              <div class="legend-item"><span class="swatch" style="background:var(--warn)"></span> 告警 <span class="l-val" id="legend-warn">0</span></div>
            </div>
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <h3>最近任务</h3>
          <button class="btn btn-ghost btn-sm" onclick="window.location='/history'">查看全部</button>
        </div>
        <div class="card-body" style="padding:0 18px">
          <ul class="task-list" id="recent-tasks">
            <li class="task-item"><div class="task-info"><div class="task-name" style="color:var(--muted)">加载中…</div></div></li>
          </ul>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <h3>快捷操作</h3>
      </div>
      <div class="card-body">
        <div class="quick-grid">
          <div class="quick-card" onclick="window.location='/exec'">
            <div class="qc-icon"><svg width="18" height="18" aria-hidden="true"><use href="#icon-terminal"/></svg></div>
            <div>
              <div class="qc-text">快速执行命令</div>
              <div class="qc-sub">在多个节点上运行</div>
            </div>
          </div>
          <div class="quick-card" onclick="window.location='/playbooks'">
            <div class="qc-icon"><svg width="18" height="18" aria-hidden="true"><use href="#icon-scroll"/></svg></div>
            <div>
              <div class="qc-text">运行剧本</div>
              <div class="qc-sub">选择预定义剧本</div>
            </div>
          </div>
          <div class="quick-card" onclick="window.location='/nodes'">
            <div class="qc-icon"><svg width="18" height="18" aria-hidden="true"><use href="#icon-plus"/></svg></div>
            <div>
              <div class="qc-text">添加节点</div>
              <div class="qc-sub">录入新受管节点</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  `, () => {
    loadAll();
  });
}
