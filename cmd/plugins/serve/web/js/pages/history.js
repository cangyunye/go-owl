export function renderHistory(render, navigate, user, api, shell) {
  let tasks = [];
  let totalTasks = 0;
  let currentPage = 1;
  const pageSize = 50;
  let wsCleanup = null;

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'s'; if (s<3600) return Math.floor(s/60)+'m'; return Math.floor(s/3600)+'h'; }

  shell.setPanelContent(`
    <li class="panel-item active"><span class="dot" style="background:var(--accent)"></span>全部 <span class="count">${totalTasks}</span></li>
    <li class="panel-item"><span class="dot" style="background:var(--success)"></span>成功</li>
    <li class="panel-item"><span class="dot" style="background:var(--danger)"></span>失败</li>
    <li class="panel-item"><span class="dot" style="background:var(--warn)"></span>进行中</li>
  `);

  async function loadTasks() {
    try {
      const res = await api.tasks({ page: currentPage, page_size: pageSize });
      tasks = res.data || [];
      totalTasks = res.meta?.total || 0;
    } catch { tasks = []; totalTasks = 0; }
    renderList();
  }

  function statusIcon(s) {
    if (s === 'completed') return '<span class="status-icon" style="background:var(--success)"></span>';
    if (s === 'failed' || s === 'cancelled') return '<span class="status-icon" style="background:var(--danger)"></span>';
    if (s === 'running') return '<span class="status-pulse" style="background:var(--warn)"></span>';
    return '<span class="status-icon" style="background:var(--muted)"></span>';
  }

  function statusText(s) {
    const map = { completed: '成功', failed: '失败', cancelled: '已取消', running: '进行中', pending: '等待中' };
    return map[s] || s;
  }

  function statusDot(s) {
    if (s === 'completed') return '<span class="hi-status success">成功</span>';
    if (s === 'failed') return '<span class="hi-status fail">失败</span>';
    if (s === 'cancelled') return '<span class="hi-status fail">已取消</span>';
    if (s === 'running') return '<span class="hi-status pending">进行中</span>';
    return '<span class="hi-status" style="background:var(--surface-2);color:var(--muted)">' + esc(s) + '</span>';
  }

  function renderList() {
    const list = document.getElementById('history-list');
    if (tasks.length === 0) {
      list.innerHTML = '<div class="view-empty" style="padding:40px"><div class="empty-title">暂无任务记录</div></div>';
    } else {
      list.innerHTML = tasks.map(t => {
        const isSuccess = t.status === 'completed';
        const isFail = t.status === 'failed' || t.status === 'cancelled';
        const isPending = t.status === 'running' || t.status === 'pending';
        return `<li class="history-item">
          <div class="hi-icon ${isSuccess ? 'success' : isFail ? 'fail' : isPending ? 'pending' : ''}">
            ${isSuccess ? '<svg width="16" height="16" aria-hidden="true"><use href="#icon-check"/></svg>' :
              isFail ? '<svg width="16" height="16" aria-hidden="true"><use href="#icon-x"/></svg>' :
              '<svg width="16" height="16" aria-hidden="true"><use href="#icon-refresh"/></svg>'}
          </div>
          <div class="hi-info">
            <div class="hi-name">${esc(t.command || t.name || '')}</div>
            <div class="hi-meta">节点: ${esc(t.node_id)} · ${t.created_at ? timeAgo(t.created_at) : '未知'}</div>
          </div>
          ${statusDot(t.status)}
          <div class="hi-action"><button class="btn btn-ghost btn-icon btn-sm" onclick="window.location='/tasks/${esc(t.id)}'" aria-label="查看详情"><svg width="14" height="14" aria-hidden="true"><use href="#icon-chevron-right"/></svg></button></div>
        </li>`;
      }).join('');
    }
    const totalPages = Math.max(1, Math.ceil(totalTasks / pageSize));
    document.getElementById('page-info').textContent = `共 ${totalTasks} 条记录`;
    document.getElementById('prev-btn').disabled = currentPage <= 1;
    document.getElementById('next-btn').disabled = currentPage >= totalPages;
  }

  render(`
    <div class="history-filters">
      <select class="select" id="status-filter">
        <option value="">全部状态</option>
        <option value="completed">成功</option>
        <option value="failed">失败</option>
        <option value="running">进行中</option>
      </select>
      <div class="spacer"></div>
      <span style="font-size:12px;color:var(--muted)" id="page-info">加载中…</span>
    </div>
    <div class="card" style="overflow:auto">
      <ul class="history-list" id="history-list" style="padding:0 18px">
        <div class="view-loading">加载中…</div>
      </ul>
    </div>
    <div style="display:flex;justify-content:center;gap:6px;padding:4px 0">
      <button class="btn btn-ghost btn-sm" id="prev-btn" disabled>‹</button>
      <button class="btn btn-ghost btn-sm" id="next-btn">›</button>
    </div>
  `, () => {
    loadTasks();

    document.getElementById('status-filter').addEventListener('change', () => {
      currentPage = 1;
      loadTasks();
    });

    document.getElementById('prev-btn').addEventListener('click', () => {
      if (currentPage > 1) { currentPage--; loadTasks(); }
    });
    document.getElementById('next-btn').addEventListener('click', () => {
      const totalPages = Math.ceil(totalTasks / pageSize);
      if (currentPage < totalPages) { currentPage++; loadTasks(); }
    });

    wsCleanup = api.connectWebSocket(msg => {
      if (msg.type === 'task_update') loadTasks();
    });
  });
}
