export function renderDashboard(render, navigate, user, api, shell) {
  let stats = { total: 0, online: 0, offline: 0, warn: 0 };
  let recentTasks = [];
  let recentAlerts = [];
  let groupStats = []; // [{group, online, offline, total}]

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'s'; if (s<3600) return Math.floor(s/60)+'m'; return Math.floor(s/3600)+'h'; }
  function alertTimeAgo(sec) { if (!sec) return '-'; const s = Math.floor(Date.now()/1000 - sec); if (s<60) return s+'秒前'; if (s<3600) return Math.floor(s/60)+'分钟前'; if (s<86400) return Math.floor(s/3600)+'小时前'; return Math.floor(s/86400)+'天前'; }
  function tagColor(s) { let h = 0; for (let i = 0; i < s.length; i++) h = ((h << 5) - h) + s.charCodeAt(i); return 'tag-r' + (Math.abs(h) % 12); }

  const ALERT_STATUS_TEXT = { open: '待处理', acked: '已确认', resolved: '已解决' };
  const ALERT_SEV_CLS = { critical: 'critical', warn: 'warn', info: 'info' };

  async function loadAll() {
    try {
      const [statsRes, taskRes, alertRes, nodes] = await Promise.all([
        api.nodeStats(),
        api.tasks({ page: 1, page_size: 5 }),
        api.alerts({ limit: 6 }).catch(() => ({ items: [] })),
        fetchAllNodes(),
      ]);
      stats.total = statsRes.total || 0;
      stats.online = statsRes.online || 0;
      stats.offline = statsRes.offline || 0;
      stats.warn = statsRes.warn || 0;
      recentTasks = taskRes.data || [];
      recentAlerts = alertRes.items || [];
      groupStats = buildGroupStats(nodes);
    } catch {}
    renderCards();
    updateTopbar();
  }

  // fetchAllNodes 分页拉全量节点（page_size 服务端上限 100，按 meta.total 翻页）。
  async function fetchAllNodes() {
    const first = await api.nodes({ page: 1, page_size: 100 }).catch(() => null);
    if (!first) return [];
    const nodes = first.data || [];
    const total = (first.meta && first.meta.total) || nodes.length;
    let page = 2;
    while (nodes.length < total && page <= 50) {
      const res = await api.nodes({ page: page++, page_size: 100 }).catch(() => null);
      if (!res || !res.data || !res.data.length) break;
      nodes.push(...res.data);
    }
    return nodes;
  }

  // buildGroupStats 按分组聚合在线/离线节点数（warn 视为在线可达）。
  function buildGroupStats(nodes) {
    const byGroup = {};
    for (const n of nodes) {
      const groups = (n.groups && n.groups.length) ? n.groups : ['未分组'];
      const online = n.status === 'online' || n.status === 'warn';
      for (const g of groups) {
        const e = byGroup[g] || (byGroup[g] = { group: g, online: 0, offline: 0 });
        if (online) e.online++; else e.offline++;
      }
    }
    return Object.values(byGroup)
      .map(e => ({ ...e, total: e.online + e.offline }))
      .sort((a, b) => b.total - a.total);
  }

  function updateTopbar() {
    const online = document.getElementById('statOnline');
    const offline = document.getElementById('statOffline');
    if (online) online.textContent = stats.online;
    if (offline) offline.textContent = stats.offline;
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

    document.getElementById('stat-total').textContent = stats.total;
    document.getElementById('stat-rate').textContent = onlineRate + '%';
    document.getElementById('stat-online').textContent = stats.online;
    document.getElementById('stat-offline').textContent = stats.offline;

    renderGroupBars();
    renderTasks();
    renderRecentAlerts();
  }

  // renderGroupBars 按分组渲染堆叠柱：上绿（在线）下灰（离线），各段标数字。
  // 段高按 maxTotal 等比缩放，非零段保底 14px 保证数字可读；渐变与配色见
  // app.css 的 gb-*（主题自适应，不用纯色）。
  function renderGroupBars() {
    const wrap = document.getElementById('group-bars');
    if (!wrap) return;
    if (!groupStats.length) {
      wrap.innerHTML = '<div class="gb-empty">暂无节点数据，录入节点后按分组展示在线 / 离线分布</div>';
      return;
    }
    const H = 150;
    const maxTotal = Math.max(...groupStats.map(g => g.total), 1);
    const segH = (v) => {
      if (!v) return 0;
      return Math.max(14, Math.round(v / maxTotal * H));
    };
    wrap.innerHTML = groupStats.map(g => {
      const hOn = segH(g.online), hOff = segH(g.offline);
      const seg = (cls, h, num) => h ? `<div class="gb-seg ${cls}" style="height:${h}px"><span class="gb-num">${num}</span></div>` : '';
      return `<div class="gb-col" title="${esc(g.group)} · 在线 ${g.online} / 离线 ${g.offline} · 共 ${g.total}">
        <div class="gb-bar" style="height:${hOn + hOff}px">
          ${seg('gb-seg-on', hOn, g.online)}
          ${seg('gb-seg-off', hOff, g.offline)}
        </div>
        <div class="gb-label">
          <span class="gb-dot ${tagColor(g.group)}" style="background:oklch(62% var(--tag-c) var(--tag-h))"></span>
          <span class="gb-name">${esc(g.group)}</span>
        </div>
      </div>`;
    }).join('');
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

  function renderRecentAlerts() {
    const list = document.getElementById('recent-alerts');
    if (!list) return;
    if (!recentAlerts.length) {
      list.innerHTML = '<li class="alert-item"><div class="al-info"><div class="al-name" style="color:var(--muted)">暂无监控记录</div><div class="al-meta">节点触发告警后将显示在这里</div></div></li>';
      return;
    }
    list.innerHTML = recentAlerts.map(a => `
      <li class="alert-item" data-alert-id="${esc(a.id)}" title="前往告警中心处理">
        <span class="al-icon ${ALERT_SEV_CLS[a.severity] || 'info'}">
          <svg width="14" height="14" aria-hidden="true"><use href="#icon-bell"/></svg>
        </span>
        <div class="al-info">
          <div class="al-name">${esc(a.message || a.alert_type_name)}</div>
          <div class="al-meta">${esc(a.alert_type_name)} · ${esc(a.node_name || a.node_id)} · ${ALERT_STATUS_TEXT[a.status] || esc(a.status)}</div>
        </div>
        <span class="al-time">${alertTimeAgo(a.first_seen)}</span>
      </li>`).join('');
    list.querySelectorAll('.alert-item[data-alert-id]').forEach(el => {
      el.addEventListener('click', () => { window.location = '/alerts'; });
    });
  }

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

    <div class="card">
      <div class="card-header">
        <h3>节点分布 · 按分组</h3>
        <div class="gb-legend">
          <span class="gb-legend-item"><span class="gb-swatch swatch-on"></span>在线</span>
          <span class="gb-legend-item"><span class="gb-swatch swatch-off"></span>离线</span>
          <span class="gb-legend-item gb-legend-note">节点属多个分组时在每组分别计数</span>
        </div>
      </div>
      <div class="card-body">
        <div class="gb-cols" id="group-bars">
          <div class="gb-empty">加载中…</div>
        </div>
      </div>
    </div>

    <div class="chart-row">
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

      <div class="card">
        <div class="card-header">
          <h3>监控记录</h3>
          <button class="btn btn-ghost btn-sm" onclick="window.location='/alerts'">查看全部</button>
        </div>
        <div class="card-body" style="padding:0 18px">
          <ul class="alert-list" id="recent-alerts">
            <li class="alert-item"><div class="al-info"><div class="al-name" style="color:var(--muted)">加载中…</div></div></li>
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
    // Hide shell panel and toggle: dashboard no longer has the overview panel
    const sidePanel = document.getElementById('sidePanel');
    const panelToggle = document.getElementById('panelToggle');
    const viewContainer = document.querySelector('.view-container');
    if (sidePanel) sidePanel.style.display = 'none';
    if (panelToggle) panelToggle.style.display = 'none';
    if (viewContainer) viewContainer.style.padding = '0';

    loadAll();
  });
}
