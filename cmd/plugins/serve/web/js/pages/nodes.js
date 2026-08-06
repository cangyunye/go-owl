export function renderNodes(render, navigate, user, api, shell) {
  let state = { nodes: [], total: 0, page: 1, pageSize: 20, query: '', status: '', filters: { groups: [], users: [] }, selectedGroups: [], groupSearch: '', pingResults: {}, checkResults: {}, selectedIds: [], expandedId: null };
  const canWrite = ['admin', 'editor', 'operator'].includes(user.role);
  const canExec = ['admin', 'operator'].includes(user.role);
  const isAdmin = user.role === 'admin';

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  const IC = {
    ping: '<svg width="15" height="15" aria-hidden="true"><use href="#icon-activity"/></svg>',
    check: '<svg width="15" height="15" aria-hidden="true"><use href="#icon-check"/></svg>',
    term: '<svg width="15" height="15" aria-hidden="true"><use href="#icon-terminal"/></svg>',
    edit: '<svg width="15" height="15" aria-hidden="true"><use href="#icon-edit"/></svg>',
    del: '<svg width="15" height="15" aria-hidden="true"><use href="#icon-x"/></svg>'
  };
  function tagColor(s) { let h = 0; for (let i = 0; i < s.length; i++) h = ((h << 5) - h) + s.charCodeAt(i); return 'tag-r' + (Math.abs(h) % 7); }
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
          <label>
            <input type="checkbox" class="group-check" value="${esc(g)}" ${checked ? 'checked' : ''}>
            <span class="dot" style="background:${checked ? 'var(--accent)' : 'var(--muted)'}"></span>
            <span class="group-text">${esc(g)}</span>
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
      list.innerHTML = '<tr><td colspan="8"><div class="view-empty" style="padding:24px"><div class="empty-title">暂无节点</div></div></td></tr>';
    } else {
      list.innerHTML = state.nodes.map(n => {
        const checked = state.selectedIds.includes(n.id);
        const groups = (n.groups || []).map(g => `<span class="tag ${tagColor(g)}">${esc(g)}</span>`).join('');
        const labels = n.labels ? Object.entries(n.labels).map(([k, v]) => `<span class="tag ${tagColor(k + ':' + v)}">${esc(k)}:${esc(v)}</span>`).join('') : '';
        const onlineLabel = n.status === 'online' ? '刚刚' : n.last_seen ? timeAgo(n.last_seen) : '-';
        const pingResult = state.pingResults[n.id];
        const checkResult = state.checkResults[n.id];
        let statusExtra = '';
        if (pingResult) {
          statusExtra = pingResult.success ? `<span style="color:var(--success);font-size:11px;margin-left:4px" title="Ping: ${pingResult.latency_ms}ms">✓${pingResult.latency_ms}ms</span>` : '<span style="color:var(--danger);font-size:11px;margin-left:4px" title="Ping failed">✗</span>';
        }
        if (checkResult) {
          statusExtra += checkResult.success ? `<span style="color:var(--success);font-size:11px;margin-left:4px" title="SSH: ${checkResult.method}">🔑</span>` : '<span style="color:var(--danger);font-size:11px;margin-left:4px" title="SSH failed">🔒</span>';
        }
        const expanded = state.expandedId === n.id;
        const caret = `<span class="expand-caret${expanded ? ' open' : ''}"><svg width="14" height="14" aria-hidden="true"><use href="#icon-chevron-right"/></svg></span>`;
        let actions = '';
        if (canWrite) {
          actions += `<button class="btn btn-ghost btn-icon btn-sm row-action" data-ping="${esc(n.id)}" title="Ping 检测" aria-label="Ping 检测">${IC.ping}</button>
            <button class="btn btn-ghost btn-icon btn-sm row-action" data-check="${esc(n.id)}" title="SSH 检查" aria-label="SSH 检查">${IC.check}</button>`;
        }
        if (canExec) {
          actions += `<button class="btn btn-ghost btn-icon btn-sm row-action" data-term="${esc(n.id)}" title="打开终端" aria-label="打开终端">${IC.term}</button>`;
        }
        if (canWrite) {
          actions += `<button class="btn btn-ghost btn-icon btn-sm row-action" data-edit="${esc(n.id)}" title="编辑节点" aria-label="编辑节点">${IC.edit}</button>`;
          if (isAdmin) {
            actions += `<button class="btn btn-ghost btn-icon btn-sm row-action danger" data-del="${esc(n.id)}" title="删除节点" aria-label="删除节点">${IC.del}</button>`;
          }
        }
        if (!actions) {
          actions = `<button class="btn btn-ghost btn-icon btn-sm row-action" data-toggle="${esc(n.id)}" title="展开详情" aria-label="展开详情"><svg width="14" height="14" aria-hidden="true"><use href="#icon-chevron-right"/></svg></button>`;
        }
        return `<tr data-toggle="${esc(n.id)}">
          <td class="checkbox-col"><input type="checkbox" class="node-check" value="${esc(n.id)}" ${checked ? 'checked' : ''} onchange="updateBatch()"></td>
          <td><div class="cell-name">${caret}<span class="node-name">${esc(n.name || n.id)}</span> <span class="sub">${esc(n.os || '')}</span></div></td>
          <td style="font-family:var(--font-mono);font-size:12px">${esc(n.address)}:${n.port}</td>
          <td>${groups ? groups : '<span style="color:var(--muted);font-size:12px">-</span>'}</td>
          <td>${statusDot(n.status)}${statusExtra}</td>
          <td><div class="cell-tags">${labels || '<span style="color:var(--muted);font-size:12px">-</span>'}</div></td>
          <td style="font-size:12px;color:var(--muted)">${onlineLabel}</td>
          <td><div class="cell-actions">${actions}</div></td>
        </tr>
        ${expanded ? `<tr class="node-expand-row">
          <td colspan="8">
            <div class="expand-detail">
              <div class="detail-field"><label>节点 ID</label><div class="value">${esc(n.id)}</div></div>
              <div class="detail-field"><label>SSH 用户</label><div class="value">${esc(n.user || '-')}</div></div>
              <div class="detail-field"><label>地址</label><div class="value" style="font-family:var(--font-mono)">${esc(n.address)}:${n.port}</div></div>
              <div class="detail-field"><label>代理跳板</label><div class="value">${n.proxy_jump ? esc(n.proxy_jump) : '<span class="value-none">无</span>'}</div></div>
              <div class="detail-field"><label>创建时间</label><div class="value">${n.created_at ? esc(new Date(n.created_at).toLocaleString()) : '-'}</div></div>
              <div class="detail-field"><label>更新时间</label><div class="value">${n.updated_at ? esc(new Date(n.updated_at).toLocaleString()) : '-'}</div></div>
            </div>
          </td>
        </tr>` : ''}`;
      }).join('');
    }
    const totalPages = Math.max(1, Math.ceil(state.total / state.pageSize));
    document.getElementById('page-info').textContent = `共 ${state.total} 条记录`;
    document.getElementById('prev-btn').disabled = state.page <= 1;
    document.getElementById('next-btn').disabled = state.page >= totalPages;
    window.updateBatch();
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

  let editTarget = null;

  function showEditModal(id) {
    editTarget = state.nodes.find(n => n.id === id);
    if (!editTarget) return;
    document.getElementById('edit-title').textContent = '编辑节点';
    document.getElementById('edit-id').value = editTarget.id;
    document.getElementById('edit-name').value = editTarget.name || '';
    document.getElementById('edit-address').value = editTarget.address || '';
    document.getElementById('edit-port').value = editTarget.port || 22;
    document.getElementById('edit-user').value = editTarget.user || '';
    document.getElementById('edit-password').value = '';
    document.getElementById('edit-sshkey').value = '';
    document.getElementById('edit-status').value = editTarget.status || 'unknown';
    document.getElementById('edit-groups').value = (editTarget.groups || []).join(', ');
    document.getElementById('edit-labels').value = Object.entries(editTarget.labels || {}).map(([k, v]) => k + ':' + v).join(', ');
    document.getElementById('edit-error').textContent = '';
    document.getElementById('edit-modal-overlay').classList.add('open');
  }

  function hideEditModal() {
    document.getElementById('edit-modal-overlay').classList.remove('open');
  }

  async function handleEditSubmit() {
    const n = editTarget;
    if (!n) return;
    const data = {};
    const name = document.getElementById('edit-name').value.trim();
    if (name !== (n.name || '')) data.name = name;
    const address = document.getElementById('edit-address').value.trim();
    if (address !== (n.address || '')) data.address = address;
    const port = parseInt(document.getElementById('edit-port').value) || 22;
    if (port !== (n.port || 22)) data.port = port;
    const user = document.getElementById('edit-user').value.trim();
    if (user !== (n.user || '')) data.user = user;
    const pw = document.getElementById('edit-password').value;
    if (pw) data.password = pw;
    const sshKey = document.getElementById('edit-sshkey').value;
    if (sshKey) data.ssh_key = sshKey;
    const status = document.getElementById('edit-status').value;
    if (status !== (n.status || 'unknown')) data.status = status;
    const groups = document.getElementById('edit-groups').value.split(',').map(s => s.trim()).filter(Boolean);
    if (JSON.stringify(groups) !== JSON.stringify(n.groups || [])) data.groups = groups;
    const labelsRaw = document.getElementById('edit-labels').value.trim();
    const labels = {};
    if (labelsRaw) {
      labelsRaw.split(',').forEach(pair => {
        const [k, ...vs] = pair.split(':');
        if (k && vs.length) labels[k.trim()] = vs.join(':').trim();
      });
    }
    if (JSON.stringify(labels) !== JSON.stringify(n.labels || {})) data.labels = labels;
    if (Object.keys(data).length === 0) { hideEditModal(); return; }

    const btn = document.getElementById('edit-save');
    btn.disabled = true; btn.textContent = '保存中…';
    try {
      await api.updateNode(n.id, data);
      hideEditModal();
      await loadNodes();
    } catch (e) { document.getElementById('edit-error').textContent = e.message; }
    btn.disabled = false; btn.textContent = '保存';
  }

  let labelMode = 'merge';

  function parseLabels(text) {
    const labels = {};
    const seen = new Set();
    const errors = [];
    if (!text.trim()) return { labels, errors };
    for (const [i, line] of text.split('\n').entries()) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      const idx = trimmed.indexOf(':');
      if (idx <= 0) { errors.push(`第 ${i + 1} 行: 缺少 ':'，格式应为 key:value`); continue; }
      const k = trimmed.slice(0, idx).trim();
      const v = trimmed.slice(idx + 1).trim();
      if (!k) { errors.push(`第 ${i + 1} 行: key 不能为空`); continue; }
      if (!v) { errors.push(`第 ${i + 1} 行: value 不能为空`); continue; }
      if (!/^[a-zA-Z0-9_.-]+$/.test(k)) { errors.push(`第 ${i + 1} 行: key "${k}" 包含非法字符，仅支持字母数字 _ . -`); continue; }
      if (seen.has(k)) { errors.push(`第 ${i + 1} 行: key "${k}" 重复`); continue; }
      seen.add(k);
      labels[k] = v;
    }
    return { labels, errors };
  }

  function showLabelModal(mode) {
    labelMode = mode || 'merge';
    const ids = getSelectedIds();
    if (ids.length === 0) return;
    const title = document.getElementById('label-modal-title');
    const desc = document.getElementById('label-mode-desc');
    if (mode === 'replace') {
      title.textContent = '同批打标签';
      desc.textContent = '所有节点将替换为完全相同的标签';
    } else {
      title.textContent = '依次打标签';
      desc.textContent = '合并到各节点，同名标签值保持一致';
    }
    if (mode === 'replace') {
      document.getElementById('label-input').value = '';
    } else {
      const allLabels = {};
      for (const n of state.nodes) {
        if (!ids.includes(n.id)) continue;
        for (const [k, v] of Object.entries(n.labels || {})) {
          if (!(k in allLabels)) allLabels[k] = v;
        }
      }
      document.getElementById('label-input').value = Object.entries(allLabels).map(([k, v]) => `${k}:${v}`).join('\n');
    }
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
    const text = document.getElementById('label-input').value;
    const { labels: newLabels, errors } = parseLabels(text);
    if (errors.length > 0) {
      document.getElementById('label-error').textContent = errors.join('; ');
      return;
    }
    const btn = document.getElementById('label-submit');
    btn.disabled = true; btn.textContent = '保存中…';
    try {
      await Promise.all(ids.map(async id => {
        const node = state.nodes.find(n => n.id === id);
        const labels = labelMode === 'replace'
          ? { ...newLabels }
          : { ...(node?.labels || {}), ...newLabels };
        return api.updateNode(id, { labels });
      }));
      hideLabelModal(); await loadNodes();
    } catch (e) { document.getElementById('label-error').textContent = e.message; }
    btn.disabled = false; btn.textContent = '确定';
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
    state.selectedIds = Array.from(document.querySelectorAll('.node-check:checked')).map(cb => cb.value);
    return state.selectedIds;
  }

  let groupModalState = { addGroups: [], removeGroups: [], currentGroups: [] };

  function showGroupModal() {
    const ids = getSelectedIds();
    if (ids.length === 0) return;
    const allGroups = new Set();
    for (const n of state.nodes) {
      if (!ids.includes(n.id)) continue;
      for (const g of (n.groups || [])) allGroups.add(g);
    }
    groupModalState = { addGroups: [], removeGroups: [], currentGroups: Array.from(allGroups).sort() };
    document.getElementById('group-node-count').textContent = ids.length;
    document.getElementById('group-error').textContent = '';
    document.getElementById('group-add-input').value = '';
    renderGroupModalTags();
    document.getElementById('group-modal-overlay').classList.add('open');
  }

  function hideGroupModal() {
    document.getElementById('group-modal-overlay').classList.remove('open');
  }

  function renderGroupModalTags() {
    const container = document.getElementById('group-tags-container');
    const currentTags = groupModalState.currentGroups
      .filter(g => !groupModalState.removeGroups.includes(g))
      .map(g => `<span class="tag ${tagColor(g)}" style="cursor:pointer;display:inline-flex;align-items:center;gap:4px" data-remove-group="${esc(g)}">${esc(g)} <span style="opacity:0.6">×</span></span>`)
      .join('');
    const addedTags = groupModalState.addGroups
      .map(g => `<span class="tag ${tagColor(g)}" style="cursor:pointer;display:inline-flex;align-items:center;gap:4px;background:var(--success);color:white" data-remove-added="${esc(g)}">${esc(g)} <span style="opacity:0.6">×</span></span>`)
      .join('');
    container.innerHTML = (currentTags || '<span style="color:var(--muted);font-size:12px">无分组</span>') + (addedTags ? ' ' + addedTags : '');
    container.querySelectorAll('[data-remove-group]').forEach(el => {
      el.addEventListener('click', () => {
        const g = el.dataset.removeGroup;
        if (!groupModalState.removeGroups.includes(g)) groupModalState.removeGroups.push(g);
        renderGroupModalTags();
      });
    });
    container.querySelectorAll('[data-remove-added]').forEach(el => {
      el.addEventListener('click', () => {
        groupModalState.addGroups = groupModalState.addGroups.filter(x => x !== el.dataset.removeAdded);
        renderGroupModalTags();
      });
    });
  }

  function addGroupFromInput() {
    const input = document.getElementById('group-add-input');
    const val = input.value.trim();
    if (!val) return;
    const newGroups = val.split(',').map(s => s.trim()).filter(Boolean);
    for (const g of newGroups) {
      if (!groupModalState.currentGroups.includes(g) && !groupModalState.addGroups.includes(g)) {
        groupModalState.addGroups.push(g);
      }
      groupModalState.removeGroups = groupModalState.removeGroups.filter(x => x !== g);
    }
    input.value = '';
    renderGroupModalTags();
  }

  async function handleGroupSubmit() {
    const ids = getSelectedIds();
    if (ids.length === 0) return;
    if (groupModalState.addGroups.length === 0 && groupModalState.removeGroups.length === 0) {
      hideGroupModal();
      return;
    }
    const btn = document.getElementById('group-submit');
    btn.disabled = true; btn.textContent = '保存中…';
    try {
      const res = await api.batchGroup(ids, { add: groupModalState.addGroups, remove: groupModalState.removeGroups });
      if (res.errors && res.errors.length > 0) {
        document.getElementById('group-error').textContent = res.errors.join('; ');
      } else {
        hideGroupModal();
        await loadNodes();
      }
    } catch (e) { document.getElementById('group-error').textContent = e.message; }
    btn.disabled = false; btn.textContent = '确定';
  }

  async function handleExport() {
    const ids = getSelectedIds();
    const params = { format: 'yaml' };
    if (ids.length > 0) params.node_ids = ids;
    if (state.selectedGroups.length > 0 && ids.length === 0) params.groups = state.selectedGroups;
    try {
      await api.exportNodes(params);
    } catch (e) { alert('导出失败: ' + e.message); }
  }

  function showImportModal() {
    document.getElementById('import-file').value = '';
    document.getElementById('import-overwrite').checked = false;
    document.getElementById('import-skip').checked = true;
    document.getElementById('import-error').textContent = '';
    document.getElementById('import-result').innerHTML = '';
    document.getElementById('import-modal-overlay').classList.add('open');
  }

  function hideImportModal() {
    document.getElementById('import-modal-overlay').classList.remove('open');
  }

  async function handleImport() {
    const fileInput = document.getElementById('import-file');
    if (!fileInput.files.length) {
      document.getElementById('import-error').textContent = '请选择文件';
      return;
    }
    const formData = new FormData();
    formData.append('file', fileInput.files[0]);
    if (document.getElementById('import-overwrite').checked) formData.append('overwrite', 'true');
    if (document.getElementById('import-skip').checked) formData.append('skip_existing', 'true');
    const btn = document.getElementById('import-submit');
    btn.disabled = true; btn.textContent = '导入中…';
    try {
      const res = await api.importNodes(formData);
      document.getElementById('import-result').innerHTML = `
        <div style="padding:12px;background:var(--surface);border-radius:var(--radius);margin-top:8px">
          <div>成功: <strong>${res.success}</strong></div>
          <div>跳过: <strong>${res.skipped}</strong></div>
          <div>失败: <strong>${res.failed}</strong></div>
          ${res.errors && res.errors.length > 0 ? `<div style="margin-top:8px;font-size:12px;color:var(--muted)">${res.errors.map(e => esc(e)).join('<br>')}</div>` : ''}
        </div>
      `;
      if (res.success > 0 && res.failed === 0) {
        await loadNodes();
      }
    } catch (e) { document.getElementById('import-error').textContent = e.message; }
    btn.disabled = false; btn.textContent = '导入';
  }

  async function handlePing() {
    const ids = getSelectedIds();
    if (ids.length === 0) return;
    state.pingResults = {};
    renderTable();
    try {
      const res = await api.pingNodes(ids);
      for (const r of res.results) {
        state.pingResults[r.node_id] = r;
      }
      renderTable();
    } catch (e) { alert('Ping 失败: ' + e.message); }
  }

  async function handleCheck() {
    const ids = getSelectedIds();
    if (ids.length === 0) return;
    state.checkResults = {};
    renderTable();
    try {
      const res = await api.checkNodes(ids);
      for (const r of res.results) {
        state.checkResults[r.node_id] = r;
      }
      renderTable();
      await loadNodes();
    } catch (e) { alert('SSH 检查失败: ' + e.message); }
  }

  async function handleRowPing(id) {
    try {
      const res = await api.pingNodes([id]);
      const r = res.results[0];
      if (r) state.pingResults[id] = r;
      renderTable();
    } catch (e) { alert('Ping 失败: ' + e.message); }
  }

  async function handleRowCheck(id) {
    try {
      const res = await api.checkNodes([id]);
      const r = res.results[0];
      if (r) state.checkResults[id] = r;
      renderTable();
      await loadNodes();
    } catch (e) { alert('SSH 检查失败: ' + e.message); }
  }

  async function handleRowDelete(id) {
    const n = state.nodes.find(x => x.id === id);
    if (!confirm(`确定删除节点 ${n ? n.name || n.id : id}？此操作不可撤销。`)) return;
    try {
      await api.deleteNode(id);
      if (state.expandedId === id) state.expandedId = null;
      await loadNodes();
    } catch (e) { alert('删除失败: ' + e.message); }
  }

  function toggleExpand(id) {
    state.expandedId = state.expandedId === id ? null : id;
    renderTable();
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
      ${canWrite ? '<button class="btn btn-secondary btn-sm" id="export-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-download"/></svg> 导出</button>' : ''}
      ${canWrite ? '<button class="btn btn-secondary btn-sm" id="import-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-upload"/></svg> 导入</button>' : ''}
      ${canWrite ? '<button class="btn btn-primary btn-sm" id="add-node-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 新建节点</button>' : ''}
    </div>

    <div class="batch-bar" id="batchBar" style="display:none">
      <span>已选 <strong id="selectedCount">0</strong> 个节点</span>
      <button class="btn btn-secondary btn-sm" id="batch-exec-btn">执行命令</button>
      <button class="btn btn-secondary btn-sm" id="batch-label-merge-btn">依次标签</button>
      <button class="btn btn-secondary btn-sm" id="batch-label-replace-btn">同批标签</button>
      <button class="btn btn-secondary btn-sm" id="batch-group-btn">管理分组</button>
      <button class="btn btn-secondary btn-sm" id="batch-ping-btn">Ping</button>
      <button class="btn btn-secondary btn-sm" id="batch-check-btn">SSH 检查</button>
      <button class="btn btn-danger btn-sm" id="batch-delete-btn">删除</button>
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

    <div class="modal-overlay" id="edit-modal-overlay" role="dialog" aria-modal="true" aria-label="编辑节点">
      <div class="modal">
        <h2 id="edit-title">编辑节点</h2>
        <div class="form-row">
          <label>节点 ID</label>
          <input type="text" class="input" id="edit-id" readonly disabled style="background:var(--bg);color:var(--muted)">
        </div>
        <div class="form-row">
          <label>节点名称</label>
          <input type="text" class="input" id="edit-name" placeholder="My Server">
        </div>
        <div class="form-row">
          <label>IP 地址 *</label>
          <input type="text" class="input" id="edit-address" placeholder="10.0.0.1">
        </div>
        <div class="form-row">
          <label>端口</label>
          <input type="number" class="input" id="edit-port" value="22">
        </div>
        <div class="form-row">
          <label>SSH 用户 *</label>
          <input type="text" class="input" id="edit-user" placeholder="root">
        </div>
        <div class="form-row">
          <label>密码</label>
          <input type="password" class="input" id="edit-password" placeholder="留空则不修改">
        </div>
        <div class="form-row">
          <label>SSH 密钥</label>
          <textarea class="input" id="edit-sshkey" placeholder="留空则不修改" rows="2" style="font-family:var(--font-mono);font-size:12px"></textarea>
        </div>
        <div class="form-row">
          <label>状态</label>
          <select class="select" id="edit-status">
            <option value="unknown">未知</option>
            <option value="online">在线</option>
            <option value="offline">离线</option>
          </select>
        </div>
        <div class="form-row">
          <label>分组</label>
          <input type="text" class="input" id="edit-groups" placeholder="web, prod">
        </div>
        <div class="form-row">
          <label>标签</label>
          <input type="text" class="input" id="edit-labels" placeholder="env:prod, tier:frontend">
        </div>
        <p class="error-msg" id="edit-error"></p>
        <div class="form-actions">
          <button class="btn btn-secondary" id="edit-cancel">取消</button>
          <button class="btn btn-primary" id="edit-save">保存</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="label-modal-overlay">
      <div class="modal modal-sm">
        <h3 id="label-modal-title">打标签</h3>
        <p style="font-size:13px;color:var(--muted);margin-bottom:12px">已选 <strong id="label-node-count">0</strong> 个节点，<span id="label-mode-desc">每行一个标签</span></p>
        <div class="form-row">
          <label>标签 (key:value)</label>
          <textarea class="input" id="label-input" rows="6" placeholder="env:production&#10;tier:frontend" style="font-family:var(--font-mono);font-size:12px;resize:vertical"></textarea>
        </div>
        <p class="error-msg" id="label-error"></p>
        <div class="form-actions">
          <button class="btn btn-secondary" id="label-cancel">取消</button>
          <button class="btn btn-primary" id="label-submit">确定</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="group-modal-overlay">
      <div class="modal modal-sm">
        <h3>管理分组</h3>
        <p style="font-size:13px;color:var(--muted);margin-bottom:12px">已选 <strong id="group-node-count">0</strong> 个节点</p>
        <div class="form-row">
          <label>当前分组 <span style="font-size:11px;color:var(--muted)">(点击移除)</span></label>
          <div id="group-tags-container" style="min-height:32px;padding:8px;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);display:flex;flex-wrap:wrap;gap:6px;align-items:center"></div>
        </div>
        <div class="form-row">
          <label>添加分组</label>
          <div style="display:flex;gap:8px">
            <input type="text" class="input" id="group-add-input" placeholder="web, prod (逗号分隔)" style="flex:1">
            <button class="btn btn-secondary btn-sm" id="group-add-btn">添加</button>
          </div>
        </div>
        <p class="error-msg" id="group-error"></p>
        <div class="form-actions">
          <button class="btn btn-secondary" id="group-cancel">取消</button>
          <button class="btn btn-primary" id="group-submit">确定</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="import-modal-overlay">
      <div class="modal modal-sm">
        <h3>导入节点</h3>
        <div class="form-row">
          <label>选择文件 (YAML/JSON)</label>
          <input type="file" class="input" id="import-file" accept=".yaml,.yml,.json">
        </div>
        <div class="form-row" style="display:flex;gap:16px">
          <label style="display:flex;align-items:center;gap:6px;cursor:pointer">
            <input type="checkbox" id="import-skip" checked style="accent-color:var(--accent)">
            <span>跳过已存在</span>
          </label>
          <label style="display:flex;align-items:center;gap:6px;cursor:pointer">
            <input type="checkbox" id="import-overwrite" style="accent-color:var(--accent)">
            <span>覆盖已存在</span>
          </label>
        </div>
        <p class="error-msg" id="import-error"></p>
        <div id="import-result"></div>
        <div class="form-actions">
          <button class="btn btn-secondary" id="import-cancel">取消</button>
          <button class="btn btn-primary" id="import-submit">导入</button>
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
      document.getElementById('export-btn').addEventListener('click', handleExport);
      document.getElementById('import-btn').addEventListener('click', showImportModal);
      document.getElementById('import-cancel').addEventListener('click', hideImportModal);
      document.getElementById('import-submit').addEventListener('click', handleImport);
      document.getElementById('import-modal-overlay').addEventListener('click', (e) => {
        if (e.target === e.currentTarget) hideImportModal();
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

    document.getElementById('batch-label-merge-btn').addEventListener('click', () => showLabelModal('merge'));
    document.getElementById('batch-label-replace-btn').addEventListener('click', () => showLabelModal('replace'));
    document.getElementById('batch-delete-btn').addEventListener('click', handleDeleteNodes);
    document.getElementById('batch-group-btn').addEventListener('click', showGroupModal);
    document.getElementById('batch-ping-btn').addEventListener('click', handlePing);
    document.getElementById('batch-check-btn').addEventListener('click', handleCheck);
    document.getElementById('batch-exec-btn').addEventListener('click', () => {
      const ids = getSelectedIds();
      const groups = state.selectedGroups;
      const params = new URLSearchParams();
      if (ids.length) params.set('nodes', ids.join(','));
      if (groups.length) params.set('groups', groups.join(','));
      window.location = '/exec?' + params.toString();
    });
    document.getElementById('label-cancel').addEventListener('click', hideLabelModal);
    document.getElementById('label-submit').addEventListener('click', handleLabelSubmit);
    document.getElementById('label-modal-overlay').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) hideLabelModal();
    });
    document.getElementById('group-cancel').addEventListener('click', hideGroupModal);
    document.getElementById('group-submit').addEventListener('click', handleGroupSubmit);
    document.getElementById('group-add-btn').addEventListener('click', addGroupFromInput);
    document.getElementById('group-add-input').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); addGroupFromInput(); }
    });
    document.getElementById('group-modal-overlay').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) hideGroupModal();
    });

    document.getElementById('node-list').addEventListener('click', (e) => {
      const toggleBtn = e.target.closest('button[data-toggle]');
      if (toggleBtn) { toggleExpand(toggleBtn.dataset.toggle); return; }
      const pingBtn = e.target.closest('[data-ping]');
      if (pingBtn) { handleRowPing(pingBtn.dataset.ping); return; }
      const checkBtn = e.target.closest('[data-check]');
      if (checkBtn) { handleRowCheck(checkBtn.dataset.check); return; }
      const termBtn = e.target.closest('[data-term]');
      if (termBtn) { navigate('/terminal/' + encodeURIComponent(termBtn.dataset.term)); return; }
      const editBtn = e.target.closest('[data-edit]');
      if (editBtn) { showEditModal(editBtn.dataset.edit); return; }
      const delBtn = e.target.closest('[data-del]');
      if (delBtn) { handleRowDelete(delBtn.dataset.del); return; }
      const row = e.target.closest('tr[data-toggle]');
      if (row && !e.target.closest('input, a, button')) {
        toggleExpand(row.dataset.toggle);
      }
    });
    document.getElementById('edit-cancel').addEventListener('click', hideEditModal);
    document.getElementById('edit-save').addEventListener('click', handleEditSubmit);
    document.getElementById('edit-modal-overlay').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) hideEditModal();
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

document.addEventListener('click', (e) => {
  if (e.target.id === 'clearSelection') {
    window.clearSelection();
  }
});
