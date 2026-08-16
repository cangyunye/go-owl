export function renderHistory(render, navigate, user, api, shell) {
  const isAdmin = user && user.role === 'admin';
  const pageSize = 50;
  const state = {
    opType: '', status: '', nodeId: '', command: '', user: '', last: '', page: 1, total: 0, records: [], stats: null, wsCleanup: null,
  };

  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'秒前'; if (s<3600) return Math.floor(s/60)+'分钟前'; if (s<86400) return Math.floor(s/3600)+'小时前'; return Math.floor(s/86400)+'天前'; }

  const OP_LABELS = { command: '命令', script: '脚本', file_transfer: '文件传输', playbook: '剧本', node_manage: '节点' };
  const OP_ICON = { command: 'terminal', script: 'terminal', file_transfer: 'upload', playbook: 'scroll', node_manage: 'nodes' };
  const STATUS_TEXT = { completed: '成功', failed: '失败', running: '进行中', cancelled: '已取消', pending: '等待中' };

  function buildParams() {
    return {
      op_type: state.opType, status: state.status, node_id: state.nodeId, command: state.command, user: state.user, last: state.last,
      limit: pageSize, offset: (state.page - 1) * pageSize,
    };
  }

  function renderPanel() {
    const st = state.stats || { total: 0, by_op_type: {} };
    const item = (key, label) => {
      const count = key === '' ? st.total : (st.by_op_type[key] || 0);
      const active = state.opType === key ? 'active' : '';
      return `<li class="panel-item ${active}" data-op="${key}"><span class="dot" style="background:var(--accent)"></span>${label} <span class="count">${count}</span></li>`;
    };
    shell.setPanelContent(
      item('', '全部') +
      item('command', '命令') +
      item('script', '脚本') +
      item('file_transfer', '文件传输') +
      item('playbook', '剧本') +
      item('node_manage', '节点')
    );
    document.querySelectorAll('#panelList .panel-item[data-op]').forEach(el => {
      el.addEventListener('click', () => { state.opType = el.dataset.op; state.page = 1; load(); });
    });
  }

  function statusBadge(s) {
    const cls = s === 'completed' ? 'success' : (s === 'failed' || s === 'cancelled') ? 'fail' : 'pending';
    return `<span class="hi-status ${cls}">${STATUS_TEXT[s] || esc(s)}</span>`;
  }

  function renderList() {
    const list = document.getElementById('history-list');
    if (!state.records.length) {
      list.innerHTML = '<div class="view-empty" style="padding:40px"><div class="empty-title">暂无历史记录</div></div>';
    } else {
      list.innerHTML = state.records.map(r => {
        const op = r.operation || {};
        const icon = OP_ICON[op.op_type] || 'clock';
        const targets = (op.targets || []).map(t => `<span class="chip">${esc(t)}</span>`).join(' ');
        return `<li class="history-item" data-task="${esc(op.task_id)}" style="cursor:pointer">
          <div class="hi-icon"><svg width="16" height="16" aria-hidden="true"><use href="#icon-${icon}"/></svg></div>
          <div class="hi-info">
            <div class="hi-name">${esc(op.command || '')}</div>
            <div class="hi-meta">${OP_LABELS[op.op_type] || esc(op.op_type)} · ${targets || '无目标'} · ${op.username ? `执行人: ${esc(op.username)} · ` : ''}${timeAgo(op.created_at)}</div>
          </div>
          ${statusBadge(op.status)}
          <div class="hi-action"><svg width="14" height="14" aria-hidden="true"><use href="#icon-chevron-right"/></svg></div>
        </li>`;
      }).join('');
      list.querySelectorAll('.history-item[data-task]').forEach(el => {
        el.addEventListener('click', () => openDetail(el.dataset.task));
      });
    }
    const totalPages = Math.max(1, Math.ceil(state.total / pageSize));
    document.getElementById('page-info').textContent = `共 ${state.total} 条记录 · 第 ${state.page}/${totalPages} 页`;
    renderPagination();
  }

  function renderPagination() {
    const container = document.getElementById('history-pagination');
    if (!container) return;
    const totalPages = Math.max(1, Math.ceil(state.total / pageSize));
    if (totalPages <= 1) { container.innerHTML = ''; return; }

    let html = '';
    html += `<button class="page-btn" data-page="${state.page - 1}" ${state.page <= 1 ? 'disabled' : ''}>◀</button>`;

    const range = 2;
    const start = Math.max(1, state.page - range);
    const end = Math.min(totalPages, state.page + range);

    if (start > 1) {
      html += `<button class="page-btn" data-page="1">1</button>`;
      if (start > 2) html += `<span class="page-ellipsis">⋯</span>`;
    }
    for (let i = start; i <= end; i++) {
      html += `<button class="page-btn ${i === state.page ? 'active' : ''}" data-page="${i}">${i}</button>`;
    }
    if (end < totalPages) {
      if (end < totalPages - 1) html += `<span class="page-ellipsis">⋯</span>`;
      html += `<button class="page-btn" data-page="${totalPages}">${totalPages}</button>`;
    }
    html += `<button class="page-btn" data-page="${state.page + 1}" ${state.page >= totalPages ? 'disabled' : ''}>▶</button>`;

    container.innerHTML = html;
    container.querySelectorAll('.page-btn:not(:disabled)').forEach(btn => {
      btn.addEventListener('click', () => {
        const p = parseInt(btn.dataset.page);
        if (p && p !== state.page) { state.page = p; load(); }
      });
    });
  }

  async function load() {
    renderPanel();
    try {
      const [listRes, statsRes] = await Promise.all([api.historyList(buildParams()), api.historyStats()]);
      state.records = listRes.data || [];
      state.total = listRes.meta?.total || 0;
      state.stats = statsRes;
    } catch { state.records = []; state.total = 0; }
    renderPanel();
    renderList();
  }

  async function openDetail(taskId) {
    let rec;
    try { rec = await api.historyGet(taskId); } catch { alert('加载详情失败'); return; }
    const op = rec.operation || {};
    const execBlocks = (rec.command_executions || []).map((e, i) => {
      const out = (e.stdout || '') + (e.stderr ? ((e.stdout ? '\n' : '') + '[stderr] ' + e.stderr) : '');
      return `<details class="exec-detail" ${i === 0 ? 'open' : ''} style="margin-bottom:6px;border:1px solid var(--border);border-radius:var(--radius);padding:8px 10px;background:var(--surface)">
        <summary style="cursor:pointer;font-size:12px;color:var(--fg)">${esc(e.node_id)} · 退出码 ${e.exit_code} · ${e.duration_ms}ms · ${e.success ? '✅ 成功' : '❌ 失败'} · <span style="font-family:var(--font-mono)">${esc(e.command)}</span></summary>
        <pre style="margin:8px 0 0;padding:10px;background:var(--bg);border-radius:var(--radius);font-family:var(--font-mono);font-size:12px;white-space:pre-wrap;word-break:break-all;color:var(--fg);max-height:320px;overflow:auto">${esc(out || '(无输出)')}</pre>
      </details>`;
    }).join('');
    const tfRows = (rec.transfers || []).map(f =>
      `<tr><td>${esc(f.node_id)}</td><td>${esc(f.file_name)}</td><td>${f.file_size || '-'}</td><td>${esc(f.transfer_type)}</td><td>${esc(f.status)}</td></tr>`).join('');
    const commRows = (rec.communications || []).map(cm =>
      `<tr><td>${esc(cm.node_id)}</td><td>${esc(cm.direction)}</td><td>${esc(cm.message_type)}</td><td>${cm.success ? '✅' : '❌'}</td></tr>`).join('');
    const isCmd = op.op_type === 'command' || op.op_type === 'script';
    const dlRow = isCmd && op.task_id
      ? `<div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:10px">
          <button class="btn btn-secondary btn-sm" id="dl-zip"><svg width="14" height="14" aria-hidden="true"><use href="#icon-download"/></svg> 下载日志 zip</button>
          ${(op.targets || []).map(n => `<button class="btn btn-ghost btn-sm" data-dl-node="${esc(n)}">${esc(n)}.log</button>`).join('')}
        </div>`
      : '';

    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay open';
    overlay.innerHTML = `<div class="modal" style="max-width:760px;max-height:80vh;overflow:auto">
      <div class="modal-header"><h3>${esc(op.command || '操作详情')}</h3>
        <button class="btn btn-ghost btn-icon" id="detail-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <p style="color:var(--muted);font-size:12px">类型: ${OP_LABELS[op.op_type] || esc(op.op_type)} · 状态: ${STATUS_TEXT[op.status] || esc(op.status)} · ${op.username ? `执行人: ${esc(op.username)} · ` : ''}时间: ${esc(op.created_at)}</p>
        <p style="color:var(--muted);font-size:12px">目标: ${(op.targets || []).map(esc).join(', ') || '无'}</p>
        ${dlRow}
        ${execBlocks ? `<h4>命令执行</h4>${execBlocks}` : ''}
        ${tfRows ? `<h4>文件传输</h4><table class="table"><thead><tr><th>节点</th><th>文件</th><th>大小</th><th>类型</th><th>状态</th></tr></thead><tbody>${tfRows}</tbody></table>` : ''}
        ${commRows ? `<h4>节点通信</h4><table class="table"><thead><tr><th>节点</th><th>方向</th><th>类型</th><th>状态</th></tr></thead><tbody>${commRows}</tbody></table>` : ''}
        ${(!execBlocks && !tfRows && !commRows) ? '<p style="color:var(--muted)">无明细数据</p>' : ''}
      </div>
    </div>`;
    document.body.appendChild(overlay);
    overlay.querySelector('#detail-close').addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
    overlay.querySelector('#dl-zip')?.addEventListener('click', () => {
      api.executionLogArchive(op.task_id).catch(e => alert('下载失败: ' + (e.message || e)));
    });
    overlay.querySelectorAll('[data-dl-node]').forEach(b => {
      b.addEventListener('click', () => {
        api.executionLogDownload(op.task_id, b.dataset.dlNode).catch(e => alert('下载失败: ' + (e.message || e)));
      });
    });
  }

  render(`
    <div class="history-filters">
      <select class="select" id="status-filter">
        <option value="">全部状态</option>
        <option value="completed">成功</option>
        <option value="failed">失败</option>
        <option value="running">进行中</option>
        <option value="cancelled">已取消</option>
      </select>
      <select class="select" id="time-filter">
        <option value="">全部时间</option>
        <option value="1h">最近 1 小时</option>
        <option value="24h">最近 24 小时</option>
        <option value="7d">最近 7 天</option>
        <option value="30d">最近 30 天</option>
      </select>
      <input class="input" id="user-filter" placeholder="按用户过滤" style="max-width:120px" />
      <input class="input" id="node-filter" placeholder="按节点过滤" style="max-width:140px" />
      <input class="input" id="cmd-filter" placeholder="搜索命令" style="flex:1;min-width:160px;max-width:280px" />
      <div class="spacer" style="flex:1"></div>
      <button class="btn btn-ghost btn-sm" id="export-json">导出 JSON</button>
      <button class="btn btn-ghost btn-sm" id="export-yaml">导出 YAML</button>
      ${isAdmin ? '<button class="btn btn-ghost btn-sm" id="clean-btn">清理</button>' : ''}
      <span style="font-size:12px;color:var(--muted)" id="page-info">加载中…</span>
    </div>
    <div class="card" style="overflow:auto">
      <ul class="history-list" id="history-list" style="padding:0 18px">
        <div class="view-loading">加载中…</div>
      </ul>
    </div>
    <div class="pagination" id="history-pagination" style="margin-top:10px"></div>
  `, () => {
    load();

    document.getElementById('status-filter').addEventListener('change', (e) => { state.status = e.target.value; state.page = 1; load(); });
    document.getElementById('time-filter').addEventListener('change', (e) => { state.last = e.target.value; state.page = 1; load(); });
    let userTimer = null;
    document.getElementById('user-filter').addEventListener('input', (e) => {
      clearTimeout(userTimer);
      userTimer = setTimeout(() => { state.user = e.target.value.trim(); state.page = 1; load(); }, 300);
    });
    let nodeTimer = null;
    document.getElementById('node-filter').addEventListener('input', (e) => {
      clearTimeout(nodeTimer);
      nodeTimer = setTimeout(() => { state.nodeId = e.target.value.trim(); state.page = 1; load(); }, 300);
    });
    let cmdTimer = null;
    document.getElementById('cmd-filter').addEventListener('input', (e) => {
      clearTimeout(cmdTimer);
      cmdTimer = setTimeout(() => { state.command = e.target.value.trim(); state.page = 1; load(); }, 300);
    });
    document.getElementById('export-json').addEventListener('click', () => api.historyExport(buildParams(), 'json').catch(() => alert('导出失败')));
    document.getElementById('export-yaml').addEventListener('click', () => api.historyExport(buildParams(), 'yaml').catch(() => alert('导出失败')));
    if (isAdmin) {
      document.getElementById('clean-btn').addEventListener('click', async () => {
        const days = prompt('清理多少天之前的历史记录？', '30');
        if (!days) return;
        const n = parseInt(days, 10);
        if (!n || n <= 0) { alert('请输入正整数天数'); return; }
        if (!confirm(`确认清理 ${n} 天之前的历史记录？此操作不可撤销。`)) return;
        try { const res = await api.historyClean(n); alert(`已清理 ${res.deleted} 条记录`); load(); }
        catch { alert('清理失败'); }
      });
    }

    state.wsCleanup = api.connectWebSocket(msg => {
      if (msg.type === 'history_update' || msg.type === 'task_update' || msg.type === 'playbook_run_update') load();
    });

    return () => { if (state.wsCleanup) state.wsCleanup.close(); };
  });
}
