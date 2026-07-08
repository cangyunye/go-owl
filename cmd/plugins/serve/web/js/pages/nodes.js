export function renderNodes(render, navigate, user, api, shell) {
  let state = { nodes: [], total: 0, page: 1, pageSize: 20, query: '', status: '', filters: { groups: [], users: [] }, selectedGroups: [], groupSearch: '' };
  const canWrite = ['admin', 'editor', 'operator'].includes(user.role);

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function statusClass(s) { return 'status-badge status-' + (s || 'unknown'); }
  function statusDot(s) {
    if (s === 'online') return '<span class="status-dot online"></span>';
    if (s === 'offline') return '<span class="status-dot offline"></span>';
    if (s === 'warn' || s === 'warning') return '<span class="status-dot warn"></span>';
    return '<span class="status-dot" style="background:var(--muted)"></span>';
  }

  async function loadFilters() {
    try {
      const res = await api.filters().catch(() => null);
      state.filters = res || { groups: [], users: [] };
    } catch {}
    const seen = {};
    for (const n of state.nodes) for (const g of (n.groups || [])) seen[g] = true;
    for (const g of ((state.filters && state.filters.groups) || [])) seen[g] = true;
    state.allGroups = Object.keys(seen).sort();
    loadPanelGroups();
  }

  function loadPanelGroups() {
    const list = document.getElementById('panelList');
    if (!list) return;
    const q = state.groupSearch.toLowerCase();
    const filtered = state.allGroups.filter(g => !q || g.toLowerCase().includes(q));
    list.innerHTML = `
      <li style="padding:6px 10px">
        <input type="text" id="groupSearchInput" placeholder="搜索分组…" style="width:100%;padding:6px 8px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface);color:var(--fg);font-size:12px;outline:none" value="${esc(state.groupSearch)}">
      </li>
      ${filtered.map(g => {
        const checked = state.selectedGroups.includes(g);
        return `<li class="panel-item" data-group="${esc(g)}" role="option" aria-selected="${checked}">
          <label style="display:flex;align-items:center;gap:8px;cursor:pointer;width:100%">
            <input type="checkbox" class="group-check" value="${esc(g)}" ${checked ? 'checked' : ''} style="accent-color:var(--accent)">
            <span class="dot" style="background:${checked ? 'var(--accent)' : 'var(--muted)'}"></span>
            <span>${esc(g)}</span>
          </label>
        </li>`;
      }).join('')}
    `;
    document.getElementById('groupSearchInput')?.addEventListener('input', onGroupSearchInput);
    document.querySelectorAll('.group-check').forEach(cb => cb.addEventListener('change', onGroupCheckChange));
  }

  async function loadNodes() {
    const params = { page: state.page, page_size: state.pageSize };
    if (state.query) params.q = state.query;
    if (state.status) params.status = state.status;
    if (state.selectedGroups.length > 0) params.group = state.selectedGroups.join(',');
    try {
      const res = state.query ? await api.searchNodes(state.query) : await api.nodes(params);
      state.nodes = (res.data || []);
      state.total = state.query ? state.nodes.length : (res.meta?.total || 0);
    } catch { state.nodes = []; state.total = 0; }
    renderTable();
    loadFilters();
  }

  function renderTable() {
    const list = document.getElementById('node-list');
    if (state.nodes.length === 0) {
      list.innerHTML = '<tr><td colspan="7"><div class="view-empty" style="padding:24px"><div class="empty-title">暂无节点</div></div></td></tr>';
    } else {
      list.innerHTML = state.nodes.map(n => {
        const groups = (n.groups || []).map(g => `<span class="tag tag-gray">${esc(g)}</span>`).join('');
        const labels = n.labels ? Object.entries(n.labels).map(([k, v]) => `<span class="tag tag-gray">${esc(k)}:${esc(v)}</span>`).join('') : '';
        const onlineLabel = n.status === 'online' ? '刚刚' : n.last_seen ? timeAgo(n.last_seen) : '-';
        return `<tr>
          <td class="checkbox-col"><input type="checkbox" class="node-check" value="${esc(n.id)}" onchange="updateBatch()"></td>
          <td><div class="cell-name"><a href="/nodes/${esc(n.id)}" style="color:var(--fg);text-decoration:none">${esc(n.name || n.id)}</a> <span class="sub">${esc(n.os || '')}</span></div></td>
          <td style="font-family:var(--font-mono);font-size:12px">${esc(n.address)}:${n.port}</td>
          <td>${groups ? groups : '<span style="color:var(--muted);font-size:12px">-</span>'}</td>
          <td>${statusDot(n.status)}</td>
          <td><div class="cell-tags">${labels || '<span style="color:var(--muted);font-size:12px">-</span>'}</div></td>
          <td style="font-size:12px;color:var(--muted)">${onlineLabel}</td>
          <td><div class="cell-actions">
            <button class="btn btn-ghost btn-icon btn-sm" onclick="window.location='/nodes/${esc(n.id)}'" aria-label="详情"><svg width="14" height="14" aria-hidden="true"><use href="#icon-chevron-right"/></svg></button>
          </div></td>
        </tr>`;
      }).join('');
    }
    const totalPages = Math.max(1, Math.ceil(state.total / state.pageSize));
    document.getElementById('page-info').textContent = `共 ${state.total} 条记录`;
    document.getElementById('prev-btn').disabled = state.page <= 1;
    document.getElementById('next-btn').disabled = state.page >= totalPages;
  }

  function timeAgo(t) {
    if (!t) return '-';
    const s = Math.floor((Date.now() - new Date(t).getTime())/1000);
    if (s < 60) return s + 's';
    if (s < 3600) return Math.floor(s/60) + 'm';
    return Math.floor(s/3600) + 'h';
  }

  let groupDebounceTimer = null;
  function onGroupSearchInput(e) {
    clearTimeout(groupDebounceTimer);
    groupDebounceTimer = setTimeout(() => {
      state.groupSearch = e.target.value;
      loadPanelGroups();
    }, 100);
  }

  function onGroupCheckChange(e) {
    const g = e.target.value;
    if (e.target.checked) {
      if (!state.selectedGroups.includes(g)) state.selectedGroups.push(g);
    } else {
      state.selectedGroups = state.selectedGroups.filter(x => x !== g);
    }
    loadPanelGroups();
    state.page = 1;
    loadNodes();
  }

  function showAddModal() {
    document.getElementById('modal-overlay').classList.add('open');
  }
  function hideAddModal() {
    document.getElementById('modal-overlay').classList.remove('open');
  }

  async function handleAdd() {
    const data = {
      id: document.getElementById('add-id').value.trim(),
      name: document.getElementById('add-name').value.trim(),
      address: document.getElementById('add-address').value.trim(),
      port: parseInt(document.getElementById('add-port').value) || 22,
      user: document.getElementById('add-user').value.trim(),
      password: document.getElementById('add-password').value || undefined,
      ssh_key: document.getElementById('add-sshkey').value || undefined,
      status: document.getElementById('add-status').value,
      groups: document.getElementById('add-groups').value.split(',').map(s => s.trim()).filter(Boolean),
      labels: {},
    };
    const labelsRaw = document.getElementById('add-labels').value.trim();
    if (labelsRaw) {
      labelsRaw.split(',').forEach(pair => {
        const [k, ...vs] = pair.split(':');
        if (k && vs.length) data.labels[k.trim()] = vs.join(':').trim();
      });
    }
    const btn = document.getElementById('add-submit');
    btn.disabled = true;
    btn.textContent = '创建中…';
    try {
      await api.createNode(data);
      hideAddModal();
      state.page = 1;
      await loadNodes();
    } catch (e) {
      document.getElementById('add-error').textContent = e.message;
    }
    btn.disabled = false;
    btn.textContent = '创建';
  }

  function showLabelModal() {
    const ids = getSelectedIds();
    if (ids.length === 0) return;
    const commonLabels = {};
    let first = true;
    for (const n of state.nodes) {
      if (!ids.includes(n.id)) continue;
      if (first) { Object.assign(commonLabels, n.labels || {}); first = false; }
      else {
        for (const key of Object.keys(commonLabels)) {
          if (!n.labels || !(key in n.labels) || n.labels[key] !== commonLabels[key])
            delete commonLabels[key];
        }
      }
    }
    document.getElementById('label-input').value = Object.entries(commonLabels).map(([k, v]) => `${k}:${v}`).join('\n');
    document.getElementById('label-node-count').textContent = ids.length;
    document.getElementById('label-error').textContent = '';
    document.getElementById('label-modal-overlay').classList.add('open');
  }

  function hideLabelModal() {
    document.getElementById('label-modal-overlay').classList.remove('open');
  }

  async function handleLabelSubmit() {
    const ids = getSelectedIds();
    if (ids.length === 0) return;
    const text = document.getElementById('label-input').value.trim();
    const newLabels = {};
    if (text) {
      for (const line of text.split('\n')) {
        const idx = line.indexOf(':');
        if (idx > 0) { const k = line.slice(0, idx).trim(), v = line.slice(idx + 1).trim(); if (k) newLabels[k] = v; }
      }
    }
    if (text && Object.keys(newLabels).length === 0) {
      document.getElementById('label-error').textContent = '格式无效，请使用 key:value 每行一个';
      return;
    }
    const btn = document.getElementById('label-submit');
    btn.disabled = true; btn.textContent = '保存中…';
    try {
      await Promise.all(ids.map(id => {
        const node = state.nodes.find(n => n.id === id);
        const merged = { ...(node?.labels || {}), ...newLabels };
        return api.updateNode(id, { labels: merged });
      }));
      hideLabelModal(); await loadNodes();
    } catch (e) { document.getElementById('label-error').textContent = e.message; }
    btn.disabled = false; btn.textContent = '保存标签';
  }

  async function handleDeleteNodes() {
    const ids = getSelectedIds();
    if (ids.length === 0) return;
    if (!confirm(`确定删除 ${ids.length} 个节点？此操作不可撤销。`)) return;
    try {
      await Promise.all(ids.map(id => api.deleteNode(id)));
      window.clearSelection(); await loadNodes();
    } catch (e) { alert('删除失败: ' + e.message); }
  }

  function getSelectedIds() {
    return Array.from(document.querySelectorAll('.node-check:checked')).map(cb => cb.value);
  }

  shell.setPanelContent('<li class="panel-item" style="cursor:default;color:var(--muted);font-size:12px">加载分组…</li>');

  loadNodes();

  render(`
    <div class="filter-bar">
      <div class="input" style="position:relative;padding-left:32px;width:240px">
        <svg width="14" height="14" aria-hidden="true" style="position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--muted)"><use href="#icon-search"/></svg>
        <input type="text" id="search-input" placeholder="搜索节点名称 / IP / 标签…" aria-label="搜索节点" style="border:none;background:transparent;outline:none;color:var(--fg);width:100%;font:13px/1.5 var(--font-body)" value="${esc(state.query)}">
      </div>
      <select class="select" id="status-filter"><option value="">全部状态</option><option value="online" ${state.status === 'online' ? 'selected' : ''}>在线</option><option value="offline" ${state.status === 'offline' ? 'selected' : ''}>离线</option><option value="warn" ${state.status === 'warn' ? 'selected' : ''}>告警</option></select>
      <div class="spacer"></div>
      ${canWrite ? '<button class="btn btn-primary btn-sm" id="add-node-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 新建节点</button>' : ''}
    </div>

    <div class="batch-bar" id="batchBar" style="display:none">
      <span>已选 <strong id="selectedCount">0</strong> 个节点</span>
      <button class="btn btn-secondary btn-sm" onclick="window.location='/exec'">执行命令</button>
      <button class="btn btn-secondary btn-sm" id="batch-label-btn">打标签</button>
      <button class="btn btn-secondary btn-sm" id="batch-delete-btn">删除</button>
      <span style="flex:1"></span>
      <button class="btn btn-ghost btn-sm" id="clearSelection">取消选择</button>
    </div>

    <div class="card" style="overflow:auto">
      <table class="data-table">
        <thead>
          <tr>
            <th class="checkbox-col"><input type="checkbox" id="checkAll" onchange="toggleAll(this)"></th>
            <th>节点名称</th>
            <th>IP 地址</th>
            <th>分组</th>
            <th>状态</th>
            <th>标签</th>
            <th>最后在线</th>
            <th></th>
          </tr>
        </thead>
        <tbody id="node-list">
          <tr><td colspan="8"><div class="view-loading">加载中…</div></td></tr>
        </tbody>
      </table>
    </div>

    <div style="display:flex;justify-content:center;gap:6px;padding:4px 0">
      <button class="btn btn-ghost btn-sm" id="prev-btn" disabled>‹</button>
      <span style="font-size:12px;color:var(--muted);padding:0 8px" id="page-info">加载中…</span>
      <button class="btn btn-ghost btn-sm" id="next-btn">›</button>
    </div>

    <div class="modal-overlay" id="modal-overlay" role="dialog" aria-modal="true" aria-label="新建节点">
      <div class="modal">
        <h2>新建节点</h2>
        <div class="form-row">
          <label>节点 ID *</label>
          <input type="text" class="input" id="add-id" placeholder="unique-id">
        </div>
        <div class="form-row">
          <label>节点名称</label>
          <input type="text" class="input" id="add-name" placeholder="My Server">
        </div>
        <div class="form-row">
          <label>IP 地址 *</label>
          <input type="text" class="input" id="add-address" placeholder="10.0.0.1">
        </div>
        <div class="form-row">
          <label>端口</label>
          <input type="number" class="input" id="add-port" value="22">
        </div>
        <div class="form-row">
          <label>SSH 用户 *</label>
          <input type="text" class="input" id="add-user" placeholder="root">
        </div>
        <div class="form-row">
          <label>密码</label>
          <input type="password" class="input" id="add-password" placeholder="可选">
        </div>
        <div class="form-row">
          <label>SSH 密钥</label>
          <textarea class="input" id="add-sshkey" placeholder="可选 SSH 私钥" rows="2" style="font-family:var(--font-mono);font-size:12px"></textarea>
        </div>
        <div class="form-row">
          <label>状态</label>
          <select class="select" id="add-status">
            <option value="unknown">未知</option>
            <option value="online">在线</option>
            <option value="offline">离线</option>
          </select>
        </div>
        <div class="form-row">
          <label>分组</label>
          <input type="text" class="input" id="add-groups" placeholder="web, prod">
        </div>
        <div class="form-row">
          <label>标签</label>
          <input type="text" class="input" id="add-labels" placeholder="env:prod, tier:frontend">
        </div>
        <p class="error-msg" id="add-error"></p>
        <div class="form-actions">
          <button class="btn btn-secondary" id="add-cancel">取消</button>
          <button class="btn btn-primary" id="add-submit">创建节点</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="label-modal-overlay">
      <div class="modal modal-sm">
        <h3>批量打标签</h3>
        <p style="font-size:13px;color:var(--muted);margin-bottom:12px">已选 <strong id="label-node-count">0</strong> 个节点，每行一个标签</p>
        <div class="form-row">
          <label>标签 (key:value)</label>
          <textarea class="input" id="label-input" rows="6" placeholder="env:production&#10;tier:frontend" style="font-family:var(--font-mono);font-size:12px;resize:vertical"></textarea>
        </div>
        <p class="error-msg" id="label-error"></p>
        <div class="form-actions">
          <button class="btn btn-secondary" id="label-cancel">取消</button>
          <button class="btn btn-primary" id="label-submit">保存标签</button>
        </div>
      </div>
    </div>
  `, () => {
    if (canWrite) {
      document.getElementById('add-node-btn').addEventListener('click', showAddModal);
      document.getElementById('add-cancel').addEventListener('click', hideAddModal);
      document.getElementById('add-submit').addEventListener('click', handleAdd);
      document.getElementById('modal-overlay').addEventListener('click', (e) => {
        if (e.target === e.currentTarget) hideAddModal();
      });
      document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
          const overlay = document.querySelector('.modal-overlay.open');
          if (overlay) overlay.classList.remove('open');
        }
      });
    }

    let searchDebounceTimer;
    document.getElementById('search-input').addEventListener('input', (e) => {
      clearTimeout(searchDebounceTimer);
      searchDebounceTimer = setTimeout(() => {
        state.query = e.target.value.trim();
        state.page = 1;
        loadNodes();
      }, 100);
    });

    document.getElementById('status-filter').addEventListener('change', (e) => {
      state.status = e.target.value;
      state.page = 1;
      loadNodes();
    });

    document.getElementById('prev-btn').addEventListener('click', () => {
      if (state.page > 1) { state.page--; loadNodes(); }
    });
    document.getElementById('next-btn').addEventListener('click', () => {
      const totalPages = Math.ceil(state.total / state.pageSize);
      if (state.page < totalPages) { state.page++; loadNodes(); }
    });

    document.getElementById('batch-label-btn').addEventListener('click', showLabelModal);
    document.getElementById('batch-delete-btn').addEventListener('click', handleDeleteNodes);
    document.getElementById('label-cancel').addEventListener('click', hideLabelModal);
    document.getElementById('label-submit').addEventListener('click', handleLabelSubmit);
    document.getElementById('label-modal-overlay').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) hideLabelModal();
    });
  });
}

window.toggleAll = function(master) {
  document.querySelectorAll('.node-check').forEach(cb => cb.checked = master.checked);
  updateBatch();
};
window.updateBatch = function() {
  const checked = document.querySelectorAll('.node-check:checked');
  const bar = document.getElementById('batchBar');
  if (bar) {
    if (checked.length > 0) {
      bar.style.display = 'flex';
      document.getElementById('selectedCount').textContent = checked.length;
    } else {
      bar.style.display = 'none';
    }
  }
};
window.clearSelection = function() {
  document.querySelectorAll('.node-check').forEach(cb => cb.checked = false);
  updateBatch();
};

// Clear selection on batch bar cancel
document.addEventListener('click', (e) => {
  if (e.target.id === 'clearSelection') {
    window.clearSelection();
  }
});
