export function renderFiles(render, navigate, user, api, shell) {
  let allNodes = [];
  let selectedNodes = new Set();
  let transfers = [];
  let transferRecords = [];
  let transferRecordTab = 'list';
  let currentPage = 1;
  const pageSize = 30;
  let totalNodes = 0;
  let allPages = 1;
  let searchQuery = '';
  let statusFilter = '';
  let activeGroups = [];
  let allGroups = [];
  let labelInputs = [];
  let stagingFiles = [];
  let diskInfo = null;
  let stagingSearch = '';
  let stagingMultiSelect = false;
  let stagingSelected = new Set();
  let transferFilter = 'all';
  let transferSearch = '';
  let transferDir = 'all';
  let startDate = '';
  let endDate = '';
  let refreshTimer = null;
  let activeDirection = 'push';

  const saved = sessionStorage.getItem('files_selected_nodes');
  if (saved) {
    try { JSON.parse(saved).forEach(id => selectedNodes.add(id)); } catch {}
  }

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'s'; if (s<3600) return Math.floor(s/60)+'m'; return Math.floor(s/3600)+'h'; }
  function tagColor(s) { let h = 0; for (let i = 0; i < s.length; i++) h = ((h << 5) - h) + s.charCodeAt(i); return 'tag-r' + (Math.abs(h) % 7); }
  function fmtSize(b) { if (!b) return '0 B'; const u = ['B','KB','MB','GB','TB']; let i = 0; let s = b; while (s >= 1024 && i < u.length-1) { s /= 1024; i++; } return s.toFixed(i > 0 ? 1 : 0) + ' ' + u[i]; }

  function saveSelection() {
    sessionStorage.setItem('files_selected_nodes', JSON.stringify(Array.from(selectedNodes)));
  }

  function updateSelectedCount() {
    const el = document.getElementById('selected-count');
    if (el) el.textContent = selectedNodes.size === 0 ? '未选择（默认全部匹配节点）' : `已选 ${selectedNodes.size} 个节点`;
  }

  function updatePathLabels() {
    const srcLabel = document.getElementById('src-label');
    const dstLabel = document.getElementById('dst-label');
    const srcInput = document.getElementById('src-path');
    const dstInput = document.getElementById('dst-path');
    const upBtn = document.getElementById('upload-btn');
    const dlBtn = document.getElementById('download-btn');
    if (!srcLabel) return;
    if (activeDirection === 'push') {
      srcLabel.textContent = '本地路径';
      dstLabel.textContent = '节点路径';
      srcInput.placeholder = '/path/to/local/file';
      dstInput.placeholder = '/path/to/remote/dir';
      upBtn.classList.add('active');
      dlBtn.classList.remove('active');
    } else {
      srcLabel.textContent = '节点路径';
      dstLabel.textContent = '本地路径';
      srcInput.placeholder = '/path/to/remote/file';
      dstInput.placeholder = '/path/to/local/dir';
      upBtn.classList.remove('active');
      dlBtn.classList.add('active');
    }
  }

  shell.setPanelContent(`
    <div class="panel-node-selector" style="display:flex;flex-direction:column;height:100%">
      <div class="panel-search">
        <input type="text" id="panel-node-search" placeholder="搜索节点名称或地址..." spellcheck="false">
      </div>
      <div class="status-filter" style="padding:0 0 6px">
        <button class="status-btn active" data-status="">全部</button>
        <button class="status-btn" data-status="online">在线</button>
        <button class="status-btn" data-status="offline">离线</button>
      </div>
      <div class="panel-node-list" id="panel-node-list">
        <span style="color:var(--muted);font-size:12px">加载中…</span>
      </div>
      <div class="panel-node-footer" id="panel-node-footer">
        <div style="display:flex;gap:4px;align-items:center">
          <button class="btn btn-ghost btn-sm" id="select-all-btn">全选</button>
          <button class="btn btn-ghost btn-sm" id="clear-selection-btn">清空</button>
          <span style="flex:1"></span>
          <span class="selected-count" id="selected-count">已选 ${selectedNodes.size} 个节点</span>
        </div>
        <div class="pagination" id="node-pagination"></div>
      </div>
    </div>
  `);

  function buildNodeQuery() {
    const opts = {};
    if (activeGroups.length) opts.group = activeGroups.join(',');
    if (statusFilter) opts.status = statusFilter;
    if (searchQuery) opts.q = searchQuery;
    const labels = labelInputs.map(l => {
      const i = l.indexOf('=');
      return i > 0 ? l.slice(0, i) + ':' + l.slice(i + 1) : null;
    }).filter(Boolean);
    if (labels.length) opts.label = labels;
    return opts;
  }

  async function loadNodes() {
    try {
      const opts = buildNodeQuery();
      opts.page = currentPage;
      opts.page_size = pageSize;
      const res = await api.nodes(opts);
      allNodes = res.data || [];
      totalNodes = res.meta?.total || 0;
      allPages = Math.ceil(totalNodes / pageSize) || 1;
      renderPanelNodeList();
      renderPagination();
    } catch { allNodes = []; }
  }

  async function selectAllFiltered() {
    try {
      const base = buildNodeQuery();
      let page = 1;
      while (true) {
        const res = await api.nodes({ ...base, page, page_size: 100 });
        const data = res.data || [];
        data.forEach(n => selectedNodes.add(n.id));
        if (data.length < 100) break;
        page++;
      }
      saveSelection();
      renderPanelNodeList();
      updateSelectedCount();
    } catch {}
  }

  function clearSelection() {
    selectedNodes.clear();
    saveSelection();
    renderPanelNodeList();
    updateSelectedCount();
  }

  function renderPanelNodeList() {
    const container = document.getElementById('panel-node-list');
    if (!container) return;
    if (allNodes.length === 0) {
      container.innerHTML = '<span style="color:var(--muted);font-size:12px">无匹配节点</span>';
      return;
    }
    container.innerHTML = allNodes.map(n => {
      const s = selectedNodes.has(n.id);
      const dot = n.status === 'online' ? 'var(--success)' : n.status === 'offline' ? 'var(--muted)' : 'var(--warn)';
      return `<span class="node-chip ${s ? 'selected' : ''}" data-id="${esc(n.id)}">
        <span class="dot" style="background:${dot}"></span>${esc(n.name || n.id)}
      </span>`;
    }).join('');
    container.querySelectorAll('.node-chip').forEach(chip => {
      chip.addEventListener('click', () => {
        const id = chip.dataset.id;
        if (selectedNodes.has(id)) { selectedNodes.delete(id); chip.classList.remove('selected'); }
        else { selectedNodes.add(id); chip.classList.add('selected'); }
        saveSelection();
        updateSelectedCount();
      });
    });
  }

  function renderPagination() {
    const container = document.getElementById('node-pagination');
    if (!container) return;
    if (allPages <= 1) { container.innerHTML = ''; return; }
    let html = '';
    html += `<button class="page-btn" data-page="${currentPage - 1}" ${currentPage <= 1 ? 'disabled' : ''}>◀</button>`;
    const range = 2;
    const start = Math.max(1, currentPage - range);
    const end = Math.min(allPages, currentPage + range);
    if (start > 1) {
      html += `<button class="page-btn" data-page="1">1</button>`;
      if (start > 2) html += `<span class="page-ellipsis">⋯</span>`;
    }
    for (let i = start; i <= end; i++) {
      html += `<button class="page-btn ${i === currentPage ? 'active' : ''}" data-page="${i}">${i}</button>`;
    }
    if (end < allPages) {
      if (end < allPages - 1) html += `<span class="page-ellipsis">⋯</span>`;
      html += `<button class="page-btn" data-page="${allPages}">${allPages}</button>`;
    }
    html += `<button class="page-btn" data-page="${currentPage + 1}" ${currentPage >= allPages ? 'disabled' : ''}>▶</button>`;
    container.innerHTML = html;
    container.querySelectorAll('.page-btn:not(:disabled)').forEach(btn => {
      btn.addEventListener('click', () => {
        const p = parseInt(btn.dataset.page);
        if (p && p !== currentPage) { currentPage = p; loadNodes(); }
      });
    });
  }

  async function loadTransfers() {
    try {
      const [tRes, rRes] = await Promise.all([api.transfers(), api.transferRecords()]);
      transfers = tRes.data || [];
      transferRecords = rRes.data || [];
    } catch { transfers = []; transferRecords = []; }
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

  function recordStatusText(s) {
    const map = { pending: '等待中', running: '传输中', partial_success: '部分成功', completed: '已完成', failed: '失败', cancelled: '已取消' };
    return map[s] || s;
  }

  function recordStatusIcon(s) {
    if (s === 'completed') return '<span class="status-icon" style="background:var(--success)"></span>';
    if (s === 'failed' || s === 'cancelled') return '<span class="status-icon" style="background:var(--danger)"></span>';
    if (s === 'partial_success') return '<span class="status-icon" style="background:var(--warn)"></span>';
    return '<span class="status-pulse" style="background:var(--warn)"></span>';
  }

  function renderTransfers() {
    const list = document.getElementById('transfer-list');
    const tabs = document.getElementById('transfer-tabs');
    if (!list) return;
    if (tabs) {
      tabs.innerHTML = `
        <div class="seg">
          <button class="${transferRecordTab === 'list' ? 'active' : ''}" data-tab="list">传输记录</button>
          <button class="${transferRecordTab === 'tasks' ? 'active' : ''}" data-tab="tasks">任务详情</button>
        </div>
      `;
      tabs.querySelectorAll('.seg button').forEach(btn => {
        btn.addEventListener('click', function() {
          tabs.querySelectorAll('.seg button').forEach(b => b.classList.remove('active'));
          this.classList.add('active');
          transferRecordTab = this.dataset.tab;
          renderTransfers();
        });
      });
    }

    if (transferRecordTab === 'tasks') {
      let filtered = [...transfers];
      if (transferFilter !== 'all') filtered = filtered.filter(t => t.status === transferFilter);
      if (transferSearch) {
        const q = transferSearch.toLowerCase();
        filtered = filtered.filter(t => (t.command || '').toLowerCase().includes(q) || t.node_id.toLowerCase().includes(q));
      }
      if (startDate) {
        const s = new Date(startDate).getTime();
        filtered = filtered.filter(t => new Date(t.created_at).getTime() >= s);
      }
      if (endDate) {
        const e = new Date(endDate).getTime() + 86400000;
        filtered = filtered.filter(t => new Date(t.created_at).getTime() <= e);
      }
      if (filtered.length === 0) {
        list.innerHTML = '<li class="task-item"><div class="task-info"><div class="task-name" style="color:var(--muted)">暂无传输任务</div></div></li>';
      } else {
        list.innerHTML = filtered.map(t => `<li class="task-item">
          ${statusIcon(t.status)}
          <div class="task-info">
            <div class="task-name">${esc(t.command || '')}</div>
            <div class="task-meta">节点: ${esc(t.node_id)} · ${t.created_at ? timeAgo(t.created_at) : ''}</div>
          </div>
          <span class="task-time">${statusText(t.status)}</span>
        </li>`).join('');
      }
      return;
    }

    if (transferRecords.length === 0) {
      list.innerHTML = '<li class="task-item"><div class="task-info"><div class="task-name" style="color:var(--muted)">暂无传输记录</div></div></li>';
    } else {
      list.innerHTML = transferRecords.map(r => {
        const stats = r.success_count + r.failed_count > 0
          ? ` · ${r.success_count}/${r.node_count} 成功`
          : ` · ${r.node_count} 节点`;
        return `<li class="task-item" data-record-id="${esc(r.id)}">
          ${recordStatusIcon(r.status)}
          <div class="task-info">
            <div class="task-name">${esc(r.file_source.split('/').pop())}</div>
            <div class="task-meta">${esc(r.dest_path)}${stats} · ${r.created_at ? timeAgo(r.created_at) : ''}</div>
          </div>
          <span class="task-time">${recordStatusText(r.status)}</span>
          <button class="btn btn-ghost btn-icon btn-sm transfer-rerun-btn" data-id="${esc(r.id)}" data-name="${esc(r.file_source.split('/').pop())}" title="重新执行" aria-label="重新执行">
            <svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg>
          </button>
        </li>`;
      }).join('');
    }
  }

  async function handleRerun(id, name) {
    if (!confirm(`确认按原参数重新执行「${name}」的传输任务？`)) return;
    try {
      const res = await api.transferRerun(id);
      alert(`重新执行已提交：${res.transfers ? res.transfers.length : '未知'} 个节点`);
      loadTransfers();
    } catch (e) {
      alert('重新执行失败: ' + (e.message || '未知错误'));
    }
  }

  function buildTransferPayload(action, src, dst) {
    const payload = {
      action: action,
      source_path: src,
      dest_path: dst,
      direction: action,
      overwrite: document.getElementById('files-overwrite')?.checked || false,
      mode: document.getElementById('files-mode')?.value.trim() || '0644',
      parallel: document.getElementById('files-parallel')?.checked ?? true,
      resume: document.getElementById('files-resume')?.checked ?? true,
    };
    if (selectedNodes.size > 0) payload.node_ids = Array.from(selectedNodes);
    if (activeGroups.length) payload.groups = activeGroups;
    if (labelInputs.length) {
      payload.labels = {};
      labelInputs.forEach(l => {
        const idx = l.indexOf('=');
        if (idx > 0) payload.labels[l.substring(0, idx)] = l.substring(idx + 1);
      });
    }
    return payload;
  }

  async function handleTransfer(action) {
    const src = document.getElementById('src-path').value.trim();
    const dst = document.getElementById('dst-path').value.trim();
    if (!src || !dst) { alert('请填写源路径和目标路径'); return; }
    try {
      const res = await api.transfer(buildTransferPayload(action, src, dst));
      if (res.transfers) {
        alert(`传输任务已提交：${res.transfers.length} 个节点`);
        loadTransfers();
      }
    } catch (e) {
      alert('传输失败: ' + (e.message || '未知错误'));
    }
  }

  async function loadFilters() {
    try {
      const res = await api.filters();
      if (res.groups) allGroups = res.groups;
      renderFilterControls();
    } catch {}
  }

  function toggleGroup(g) {
    const idx = activeGroups.indexOf(g);
    if (idx >= 0) { activeGroups.splice(idx, 1); }
    else { activeGroups.push(g); }
    renderGroupChips();
    currentPage = 1;
    loadNodes();
  }

  function renderGroupChips() {
    const container = document.getElementById('files-group-chips');
    if (!container) return;
    container.innerHTML = allGroups.map(g => {
      const active = activeGroups.includes(g);
      return `<span class="node-chip ${active ? 'selected' : ''}" data-group="${esc(g)}">${esc(g)}</span>`;
    }).join('');
    container.querySelectorAll('.node-chip').forEach(chip => {
      chip.addEventListener('click', () => toggleGroup(chip.dataset.group));
    });
  }

  function addLabel() {
    const input = document.getElementById('files-label-input');
    const val = input.value.trim();
    if (!val || !val.includes('=')) return;
    if (!labelInputs.includes(val)) {
      labelInputs.push(val);
      input.value = '';
      renderLabelTags();
      currentPage = 1;
      loadNodes();
    }
  }

  function removeLabel(l) {
    labelInputs = labelInputs.filter(x => x !== l);
    renderLabelTags();
    currentPage = 1;
    loadNodes();
  }

  function renderLabelTags() {
    const container = document.getElementById('files-label-tags');
    if (!container) return;
    container.innerHTML = labelInputs.map(l =>
      `<span class="tag tag-blue">${esc(l)} <span class="label-remove" data-label="${esc(l)}" style="cursor:pointer;margin-left:2px">×</span></span>`
    ).join('');
    document.querySelectorAll('.label-remove').forEach(el => {
      el.addEventListener('click', function() { removeLabel(this.dataset.label); });
    });
  }

  function renderFilterControls() {
    const container = document.getElementById('files-filter-controls');
    if (!container) return;
    container.innerHTML = `
      <div class="filter-row">
        <label>分组</label>
        <div style="display:flex;gap:4px;flex-wrap:wrap" id="files-group-chips"></div>
      </div>
      <div class="filter-row">
        <label>标签</label>
        <div style="display:flex;gap:4px;flex-wrap:wrap" id="files-label-tags"></div>
        <div style="display:flex;gap:4px;margin-top:4px">
          <input type="text" id="files-label-input" class="exec-input" placeholder="key=value" style="flex:1;min-width:0">
          <button class="btn btn-ghost btn-sm" id="files-add-label-btn">+</button>
        </div>
      </div>
      <div class="filter-row" style="margin-top:12px;border-top:1px solid var(--border);padding-top:12px">
        <label>传输选项</label>
        <label class="toggle-row" style="margin-top:4px">
          <input type="checkbox" id="files-overwrite">
          <span class="toggle-track"><span class="toggle-thumb"></span></span>
          <span style="font-size:12px;color:var(--muted)">覆盖已有文件</span>
        </label>
        <div style="display:flex;align-items:center;gap:6px;margin-top:6px">
          <span style="font-size:11px;color:var(--muted)">权限</span>
          <input type="text" id="files-mode" class="exec-input" value="0644" style="width:60px;text-align:center">
        </div>
        <label class="toggle-row" style="margin-top:4px">
          <input type="checkbox" id="files-parallel" checked>
          <span class="toggle-track"><span class="toggle-thumb"></span></span>
          <span style="font-size:12px;color:var(--muted)">并行传输</span>
        </label>
        <label class="toggle-row" style="margin-top:4px">
          <input type="checkbox" id="files-resume" checked>
          <span class="toggle-track"><span class="toggle-thumb"></span></span>
          <span style="font-size:12px;color:var(--muted)">断点续传</span>
        </label>
      </div>
    `;
    renderGroupChips();
    renderLabelTags();
    document.getElementById('files-add-label-btn').addEventListener('click', addLabel);
    document.getElementById('files-label-input').addEventListener('keydown', e => { if (e.key === 'Enter') addLabel(); });
  }

  async function loadStaging() {
    try {
      const [fRes, dRes] = await Promise.all([api.staging.files(), api.staging.disk()]);
      stagingFiles = fRes.data || [];
      diskInfo = dRes;
    } catch { stagingFiles = []; diskInfo = null; }
    renderStaging();
  }

  function fmtTime(t) {
    if (!t) return '-';
    const d = new Date(t);
    return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function renderStaging() {
    const list = document.getElementById('staging-file-list');
    const bar = document.getElementById('staging-disk-bar');
    const info = document.getElementById('staging-disk-info');
    if (!list) return;
    if (diskInfo && bar) {
      const pct = diskInfo.total > 0 ? (diskInfo.used / diskInfo.total * 100) : 0;
      const barColor = diskInfo.free < diskInfo.threshold ? 'var(--danger)' : 'var(--success)';
      bar.innerHTML = `<div style="height:100%;width:${Math.min(pct, 100)}%;background:${barColor};border-radius:4px;transition:width 0.3s"></div>`;
    }
    if (info && diskInfo) {
      info.textContent = `使用 ${fmtSize(diskInfo.used)} / ${fmtSize(diskInfo.total)} · 剩余 ${fmtSize(diskInfo.free)}`;
    }
    const filtered = stagingSearch
      ? stagingFiles.filter(f => f.name.toLowerCase().includes(stagingSearch.toLowerCase()))
      : stagingFiles;
    const canDelete = user && user.role === 'admin';
    if (filtered.length === 0) {
      list.innerHTML = '<div class="staging-empty">暂无文件</div>';
      return;
    }
    const stagingDir = diskInfo ? diskInfo.staging_dir : '';
    const showCheck = stagingMultiSelect ? '' : 'display:none';
    const allChecked = filtered.length > 0 && filtered.every(f => stagingSelected.has(f.name));
    list.innerHTML = `<table class="staging-table">
      <thead><tr>
        <th class="stg-ck" style="${showCheck}"><input type="checkbox" class="staging-select-all" ${allChecked ? 'checked' : ''}></th>
        <th class="stg-name">文件名</th>
        <th class="stg-path">路径</th>
        <th class="stg-time">创建时间</th>
        <th class="stg-size">大小</th>
        ${canDelete ? '<th class="stg-act"></th>' : ''}
      </tr></thead>
      <tbody>${filtered.map(f => {
        const fullPath = stagingDir ? stagingDir + '/' + f.name : f.name;
        const fileTime = f.create_time || f.mod_time || '';
        const checked = stagingSelected.has(f.name) ? 'checked' : '';
        return `<tr class="staging-file-row" data-name="${esc(f.name)}">
          <td class="stg-ck" style="${showCheck}"><input type="checkbox" class="staging-checkbox" data-name="${esc(f.name)}" ${checked}></td>
          <td class="stg-name" title="${esc(f.name)}">${esc(f.name)}</td>
          <td class="stg-path" title="${esc(fullPath)}">${esc(fullPath)}</td>
          <td class="stg-time">${fmtTime(fileTime)}</td>
          <td class="stg-size">${fmtSize(f.size)}</td>
          ${canDelete ? `<td class="stg-act"><button class="btn btn-ghost btn-icon btn-sm staging-delete-btn" data-name="${esc(f.name)}" title="删除"><svg width="14" height="14" aria-hidden="true" style="color:var(--danger)"><use href="#icon-x"/></svg></button></td>` : ''}
        </tr>`;
      }).join('')}</tbody>
    </table>`;

    const selectAll = list.querySelector('.staging-select-all');
    if (selectAll) {
      selectAll.addEventListener('change', function() {
        filtered.forEach(f => {
          if (this.checked) stagingSelected.add(f.name);
          else stagingSelected.delete(f.name);
        });
        list.querySelectorAll('.staging-checkbox').forEach(cb => { cb.checked = this.checked; });
        const batchBtn = document.getElementById('staging-batch-btn');
        if (batchBtn) {
          const count = stagingSelected.size;
          batchBtn.textContent = count ? `批量传输 (${count})` : '批量传输';
          batchBtn.style.display = stagingMultiSelect && count > 0 ? 'inline-flex' : 'none';
        }
      });
    }
    list.querySelectorAll('.staging-delete-btn').forEach(btn => {
      btn.addEventListener('click', async function(e) {
        e.stopPropagation();
        const name = this.dataset.name;
        if (!confirm(`确认删除 ${name}？`)) return;
        try {
          await api.staging.delete(name);
          loadStaging();
        } catch (e) { alert('删除失败: ' + e.message); }
      });
    });
    list.querySelectorAll('.staging-file-row').forEach(row => {
      row.addEventListener('click', function(e) {
        if (e.target.closest('.staging-delete-btn') || e.target.closest('.staging-checkbox')) return;
        const name = this.dataset.name;
        const stagingDir = diskInfo ? diskInfo.staging_dir : '';
        const fullPath = stagingDir ? stagingDir + '/' + name : name;
        const srcInput = document.getElementById('src-path');
        if (srcInput) {
          srcInput.value = fullPath;
          srcInput.focus();
        }
      });
    });
    list.querySelectorAll('.staging-checkbox').forEach(cb => {
      cb.addEventListener('change', function() {
        const name = this.dataset.name;
        if (this.checked) stagingSelected.add(name);
        else stagingSelected.delete(name);
        const batchBtn = document.getElementById('staging-batch-btn');
        if (batchBtn) {
          const count = stagingSelected.size;
          batchBtn.textContent = count ? `批量传输 (${count})` : '批量传输';
          batchBtn.style.display = stagingMultiSelect && count > 0 ? 'inline-flex' : 'none';
        }
        if (selectAll) {
          selectAll.checked = filtered.every(f => stagingSelected.has(f.name));
        }
      });
    });
  }

  async function handleStagingUpload(file) {
    if (!file) return;
    try {
      await api.staging.upload(file);
      loadStaging();
    } catch (e) {
      alert('上传失败: ' + (e.message || '未知错误'));
    }
  }

  async function handleStagingUploads(files) {
    if (!files || !files.length) return;
    let success = 0;
    let fail = 0;
    for (const file of files) {
      try {
        await api.staging.upload(file);
        success++;
      } catch (e) {
        fail++;
        console.error('上传失败', file.name, e);
      }
    }
    if (files.length > 1) {
      alert(`上传完成：${success} 成功, ${fail} 失败`);
    }
    loadStaging();
  }

  function startAutoRefresh() {
    if (refreshTimer) clearInterval(refreshTimer);
    refreshTimer = setInterval(loadTransfers, 5000);
  }

  function stopAutoRefresh() {
    if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }
  }

  render(`
    <div class="exec-layout">
      <div class="exec-main">
        <div class="card">
          <div class="card-header"><h3>文件传输</h3></div>
          <div class="card-body">
            <div style="display:grid;grid-template-columns:1fr auto 1fr;gap:14px;align-items:start">
              <div>
                <label id="src-label" style="font-size:12px;color:var(--muted);display:block;margin-bottom:6px">本地路径</label>
                <input type="text" class="input" id="src-path" style="width:100%" value="/var/log/app/debug.log" placeholder="/path/to/local/file">
              </div>
              <div style="display:grid;place-items:center;padding-top:18px">
                <svg width="24" height="24" aria-hidden="true" style="color:var(--accent)"><use href="#icon-upload"/></svg>
              </div>
              <div>
                <label id="dst-label" style="font-size:12px;color:var(--muted);display:block;margin-bottom:6px">节点路径</label>
                <input type="text" class="input" id="dst-path" style="width:100%" value="/tmp/logs/" placeholder="/path/to/remote/dir">
              </div>
            </div>
            <div style="margin-top:14px;display:flex;gap:8px">
              <button class="btn btn-primary active" id="upload-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-upload"/></svg> 上传</button>
              <button class="btn btn-secondary" id="download-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg> 下载</button>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3>传输记录</h3></div>
          <div class="card-body" style="padding:8px 14px 0">
            <div style="display:flex;gap:6px;margin-bottom:8px" id="transfer-tabs"></div>
            <div style="display:flex;gap:8px;flex-wrap:wrap;margin-bottom:8px;align-items:center">
              <div class="seg">
                <button class="active" data-tf="all">全部</button>
                <button data-tf="completed">成功</button>
                <button data-tf="failed">失败</button>
                <button data-tf="running">进行中</button>
              </div>
              <span style="flex:1"></span>
              <input type="text" id="transfer-search" class="exec-input" placeholder="搜索文件名/节点..." style="width:160px">
            </div>
            <div style="display:flex;gap:8px;margin-bottom:8px;align-items:center">
              <label style="font-size:11px;color:var(--muted)">起始</label>
              <input type="date" id="tf-start-date" class="exec-input" style="width:140px">
              <label style="font-size:11px;color:var(--muted)">截止</label>
              <input type="date" id="tf-end-date" class="exec-input" style="width:140px">
            </div>
          </div>
          <div class="card-body" style="padding:0">
            <ul class="task-list" id="transfer-list" style="padding:0 18px">
              <li class="task-item"><div class="task-info"><div class="task-name" style="color:var(--muted)">加载中…</div></div></li>
            </ul>
          </div>
        </div>

        <div class="card">
          <div class="card-header" style="display:flex;align-items:center;gap:8px">
            <h3 style="flex:1">文件中转站</h3>
            <input type="text" id="staging-search" class="exec-input" placeholder="搜索文件名..." style="width:140px;font-size:12px">
          </div>
          <div class="card-body staging-dropzone" id="staging-dropzone">
            <div class="staging-drop-overlay" id="staging-drop-overlay">
              <div style="text-align:center">
                <div style="font-size:14px;font-weight:600;margin-bottom:4px">松开以上传文件</div>
                <div style="font-size:12px;color:var(--muted)">支持拖放多个文件到中转站</div>
              </div>
            </div>
            <div style="display:flex;align-items:center;gap:10px;margin-bottom:10px">
              <div style="flex:1;min-width:0">
                <div id="staging-disk-bar" style="height:8px;background:var(--border);border-radius:4px;overflow:hidden"></div>
                <div id="staging-disk-info" style="font-size:11px;color:var(--muted);margin-top:2px">加载中…</div>
              </div>
              <button class="btn btn-ghost btn-sm" id="staging-multi-btn" style="white-space:nowrap;flex-shrink:0" title="多选模式">
                <svg width="13" height="13" aria-hidden="true" style="margin-right:3px;vertical-align:-2px"><use href="#icon-check"/></svg>
                多选
              </button>
              <button class="btn btn-secondary btn-sm" id="staging-pick-btn" style="white-space:nowrap;flex-shrink:0">
                <svg width="13" height="13" aria-hidden="true" style="margin-right:3px;vertical-align:-2px"><use href="#icon-plus"/></svg>
                选择
              </button>
              <input type="file" id="staging-file-input" hidden>
              <div style="width:100px;flex-shrink:0">
                <button class="btn btn-primary btn-sm" id="staging-upload-btn" disabled style="width:100%;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">上传中转站</button>
                <button class="btn btn-primary btn-sm" id="staging-batch-btn" style="width:100%;white-space:nowrap;display:none">批量传输</button>
              </div>
            </div>
            <div id="staging-file-list" style="max-height:280px;overflow-y:auto">加载中…</div>
          </div>
        </div>
      </div>

      <div class="exec-sidebar">
        <div class="card">
          <div class="card-header"><h3>筛选条件</h3></div>
          <div class="card-body" id="files-filter-controls"></div>
        </div>
      </div>
    </div>
  `, () => {
    loadNodes();
    loadTransfers();
    loadFilters();
    loadStaging();
    startAutoRefresh();
    updateSelectedCount();

    updatePathLabels();

    document.getElementById('upload-btn').addEventListener('click', () => {
      activeDirection = 'push';
      updatePathLabels();
      handleTransfer('push');
    });
    document.getElementById('download-btn').addEventListener('click', () => {
      activeDirection = 'pull';
      updatePathLabels();
      handleTransfer('pull');
    });

    document.getElementById('panel-node-search').addEventListener('input', function() {
      searchQuery = this.value.trim();
      currentPage = 1;
      loadNodes();
    });

    document.getElementById('select-all-btn').addEventListener('click', selectAllFiltered);
    document.getElementById('clear-selection-btn').addEventListener('click', clearSelection);

    document.querySelectorAll('.status-btn[data-status]').forEach(btn => {
      btn.addEventListener('click', function() {
        document.querySelectorAll('.status-btn[data-status]').forEach(b => b.classList.remove('active'));
        this.classList.add('active');
        statusFilter = this.dataset.status;
        currentPage = 1;
        selectedNodes.clear();
        saveSelection();
        loadNodes();
        updateSelectedCount();
      });
    });

    document.querySelectorAll('.seg button[data-tf]').forEach(btn => {
      btn.addEventListener('click', function() {
        document.querySelectorAll('.seg button[data-tf]').forEach(b => b.classList.remove('active'));
        this.classList.add('active');
        transferFilter = this.dataset.tf;
        renderTransfers();
      });
    });

    document.getElementById('transfer-search').addEventListener('input', function() {
      transferSearch = this.value.trim();
      renderTransfers();
    });

    document.getElementById('tf-start-date').addEventListener('change', function() {
      startDate = this.value;
      renderTransfers();
    });

    document.getElementById('tf-end-date').addEventListener('change', function() {
      endDate = this.value;
      renderTransfers();
    });

    document.getElementById('staging-pick-btn').addEventListener('click', () => {
      document.getElementById('staging-file-input').click();
    });

    document.getElementById('staging-file-input').addEventListener('change', function() {
      const btn = document.getElementById('staging-upload-btn');
      btn.disabled = !this.files.length;
      if (this.files.length) {
        btn.textContent = '上传中转站';
      } else {
        btn.textContent = '上传中转站';
      }
    });

    document.getElementById('staging-upload-btn').addEventListener('click', () => {
      const input = document.getElementById('staging-file-input');
      handleStagingUpload(input.files[0]);
      input.value = '';
      document.getElementById('staging-upload-btn').textContent = '上传中转站';
      document.getElementById('staging-upload-btn').disabled = true;
    });

    document.getElementById('staging-search').addEventListener('input', function() {
      stagingSearch = this.value.trim();
      renderStaging();
    });

    const dropzone = document.getElementById('staging-dropzone');
    let dragDepth = 0;
    dropzone.addEventListener('dragenter', e => {
      if (!e.dataTransfer || !e.dataTransfer.types.includes('Files')) return;
      e.preventDefault();
      dragDepth++;
      dropzone.classList.add('staging-drag-over');
    });
    dropzone.addEventListener('dragover', e => {
      if (!e.dataTransfer || !e.dataTransfer.types.includes('Files')) return;
      e.preventDefault();
    });
    dropzone.addEventListener('dragleave', e => {
      dragDepth = Math.max(0, dragDepth - 1);
      if (dragDepth === 0) dropzone.classList.remove('staging-drag-over');
    });
    dropzone.addEventListener('drop', e => {
      e.preventDefault();
      dragDepth = 0;
      dropzone.classList.remove('staging-drag-over');
      const files = e.dataTransfer ? Array.from(e.dataTransfer.files || []) : [];
      if (files.length) handleStagingUploads(files);
    });

document.getElementById('staging-multi-btn').addEventListener('click', function() {
    stagingMultiSelect = !stagingMultiSelect;
    if (!stagingMultiSelect) stagingSelected.clear();
    this.classList.toggle('active');
    const srcInput = document.getElementById('src-path');
    srcInput.disabled = stagingMultiSelect;
    if (stagingMultiSelect) srcInput.value = '';
    document.getElementById('staging-upload-btn').style.display = stagingMultiSelect ? 'none' : 'inline-flex';
    document.getElementById('staging-batch-btn').style.display = stagingMultiSelect ? 'inline-flex' : 'none';
    renderStaging();
  });

  document.getElementById('staging-batch-btn').addEventListener('click', async function() {
    if (stagingSelected.size === 0) return;
    const dstInput = document.getElementById('dst-path');
    const dst = dstInput ? dstInput.value.trim() : '';
    if (!dst) { alert('请填写节点路径'); return; }
    const stagingDir = diskInfo ? diskInfo.staging_dir : '';
    let success = 0, fail = 0;
    for (const name of stagingSelected) {
      const fullPath = stagingDir ? stagingDir + '/' + name : name;
      try {
        await api.transfer(buildTransferPayload('push', fullPath, dst));
        success++;
      } catch (e) {
        fail++;
      }
    }
    alert(`批量传输完成：${success} 成功, ${fail} 失败`);
    stagingSelected.clear();
    stagingMultiSelect = false;
    document.getElementById('staging-multi-btn').classList.remove('active');
    document.getElementById('src-path').disabled = false;
    document.getElementById('staging-upload-btn').style.display = 'inline-flex';
    document.getElementById('staging-batch-btn').style.display = 'none';
    loadTransfers();
    renderStaging();
  });

  document.getElementById('transfer-list').addEventListener('click', function(e) {
    const btn = e.target.closest('.transfer-rerun-btn');
    if (!btn) return;
    e.stopPropagation();
    handleRerun(btn.dataset.id, btn.dataset.name);
  });
  });
}
