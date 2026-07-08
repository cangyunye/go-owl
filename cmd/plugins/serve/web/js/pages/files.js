export function renderFiles(render, navigate, user, api, shell) {
  let allNodes = [];
  let selectedNodes = new Set();
  let transfers = [];

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'s'; if (s<3600) return Math.floor(s/60)+'m'; return Math.floor(s/3600)+'h'; }

  shell.setPanelContent(`
    <li class="panel-item active"><span class="dot" style="background:var(--accent)"></span>全部记录</li>
    <li class="panel-item"><span class="dot" style="background:var(--success)"></span>成功</li>
    <li class="panel-item"><span class="dot" style="background:var(--danger)"></span>失败</li>
  `);

  async function loadNodes() {
    try {
      const res = await api.nodes({ page: 1, page_size: 200 });
      allNodes = res.data || [];
      renderNodeChips();
    } catch { allNodes = []; }
  }

  function renderNodeChips() {
    const container = document.getElementById('file-node-chips');
    if (!container) return;
    container.innerHTML = allNodes.map(n => {
      const selected = selectedNodes.has(n.id);
      const dotColor = n.status === 'online' ? 'var(--success)' : 'var(--muted)';
      return `<span class="node-chip ${selected ? 'selected' : ''}" data-id="${esc(n.id)}">
        <span class="dot" style="background:${dotColor}"></span>${esc(n.name || n.id)}
      </span>`;
    }).join('');
    container.querySelectorAll('.node-chip').forEach(chip => {
      chip.addEventListener('click', () => {
        const id = chip.dataset.id;
        if (selectedNodes.has(id)) { selectedNodes.delete(id); chip.classList.remove('selected'); }
        else { selectedNodes.add(id); chip.classList.add('selected'); }
      });
    });
  }

  async function loadTransfers() {
    try {
      const res = await api.transfers();
      transfers = res.data || [];
    } catch { transfers = []; }
    renderTransfers();
  }

  function statusIcon(s) {
    if (s === 'completed') return '<span class="status-icon" style="background:var(--success)"></span>';
    if (s === 'failed' || s === 'cancelled') return '<span class="status-icon" style="background:var(--danger)"></span>';
    return '<span class="status-pulse" style="background:var(--warn)"></span>';
  }

  function statusText(s) {
    const map = { completed: '完成', failed: '失败', cancelled: '已取消', running: '进行中', pending: '等待中' };
    return map[s] || s;
  }

  function renderTransfers() {
    const list = document.getElementById('transfer-list');
    if (!list) return;
    if (transfers.length === 0) {
      list.innerHTML = '<li class="task-item"><div class="task-info"><div class="task-name" style="color:var(--muted)">暂无传输记录</div></div></li>';
    } else {
      list.innerHTML = transfers.map(t => `<li class="task-item">
        ${statusIcon(t.status)}
        <div class="task-info">
          <div class="task-name">${esc(t.command || '')}</div>
          <div class="task-meta">节点: ${esc(t.node_id)} · ${t.created_at ? timeAgo(t.created_at) : ''}</div>
        </div>
        <span class="task-time">${statusText(t.status)}</span>
      </li>`).join('');
    }
  }

  async function handleTransfer(action) {
    const src = document.getElementById('src-path').value.trim();
    const dst = document.getElementById('dst-path').value.trim();
    if (!src || !dst) { alert('请填写源路径和目标路径'); return; }
    if (selectedNodes.size === 0) { alert('请选择目标节点'); return; }

    try {
      const res = await api.transfer({
        action: action,
        node_ids: Array.from(selectedNodes),
        source_path: src,
        dest_path: dst,
        direction: action,
      });
      if (res.transfers) {
        alert(`传输任务已提交：${res.transfers.length} 个节点`);
        loadTransfers();
      }
    } catch (e) {
      alert('传输失败: ' + (e.message || '未知错误'));
    }
  }

  render(`
    <div class="card">
      <div class="card-header"><h3>文件传输</h3></div>
      <div class="card-body">
        <div style="display:grid;grid-template-columns:1fr auto 1fr;gap:14px;align-items:start">
          <div>
            <label style="font-size:12px;color:var(--muted);display:block;margin-bottom:6px">源路径</label>
            <input type="text" class="input" id="src-path" style="width:100%" value="/var/log/app/debug.log" placeholder="/path/to/source">
          </div>
          <div style="display:grid;place-items:center;padding-top:18px">
            <svg width="24" height="24" aria-hidden="true" style="color:var(--accent)"><use href="#icon-upload"/></svg>
          </div>
          <div>
            <label style="font-size:12px;color:var(--muted);display:block;margin-bottom:6px">目标路径</label>
            <input type="text" class="input" id="dst-path" style="width:100%" value="/tmp/logs/" placeholder="/path/to/destination">
          </div>
        </div>
        <div style="margin-top:14px">
          <label style="font-size:12px;color:var(--muted);display:block;margin-bottom:6px">目标节点</label>
          <div class="node-selector" id="file-node-chips">
            <span style="color:var(--muted);font-size:12px">加载中…</span>
          </div>
        </div>
        <div style="margin-top:14px;display:flex;gap:8px">
          <button class="btn btn-primary" id="upload-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-upload"/></svg> 上传</button>
          <button class="btn btn-secondary" id="download-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg> 下载</button>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-header"><h3>传输记录</h3></div>
      <div class="card-body" style="padding:0">
        <ul class="task-list" id="transfer-list" style="padding:0 18px">
          <li class="task-item"><div class="task-info"><div class="task-name" style="color:var(--muted)">加载中…</div></div></li>
        </ul>
      </div>
    </div>
  `, () => {
    loadNodes();
    loadTransfers();
    document.getElementById('upload-btn').addEventListener('click', () => handleTransfer('push'));
    document.getElementById('download-btn').addEventListener('click', () => handleTransfer('pull'));
  });
}
