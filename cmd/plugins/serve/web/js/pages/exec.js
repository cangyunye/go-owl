export function renderExec(render, navigate, user, api, shell) {
  let allNodes = [];
  let selectedNodes = new Set();
  let wsCleanup = null;
  let currentTaskIDs = [];
  let currentTasks = [];
  let currentOpID = '';
  let activeGroups = [];
  let allGroups = [];
  let allLabels = [];
  let labelInputs = [];
  let statusFilter = '';
  let currentPage = 1;
  const pageSize = 30;
  let totalNodes = 0;
  let allPages = 1;
  let searchQuery = '';
  let execMode = 'command';
  let scriptInputMode = 'inline';
  let scriptFileContent = '';
  let scriptFileName = '';
  let commandContent = 'uptime\ndf -h\nfree -m';
  let scriptInlineContent = '';
  let shortcuts = [];
  let editingShortcutID = null;

  const params = new URLSearchParams(window.location.search);
  const initNodes = params.get('nodes');
  const initGroups = params.get('groups');

  const saved = sessionStorage.getItem('exec_selected_nodes');
  if (saved) {
    try { JSON.parse(saved).forEach(id => selectedNodes.add(id)); } catch {}
  }

  if (initGroups) activeGroups = initGroups.split(',').filter(Boolean);
  if (initNodes) initNodes.split(',').filter(Boolean).forEach(id => selectedNodes.add(id));

  function saveSelection() {
    sessionStorage.setItem('exec_selected_nodes', JSON.stringify(Array.from(selectedNodes)));
  }

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  function tagColor(s) { let h = 0; for (let i = 0; i < s.length; i++) h = ((h << 5) - h) + s.charCodeAt(i); return 'tag-r' + (Math.abs(h) % 7); }

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
          <span class="selected-count" id="selected-count">已选 0 个节点</span>
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
      updateExecButton();
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
      updateSelectedCount();
      updateExecButton();
      renderPanelNodeList();
    } catch {}
  }

  function clearSelection() {
    selectedNodes.clear();
    saveSelection();
    updateSelectedCount();
    updateExecButton();
    renderPanelNodeList();
  }

  function renderPanelNodeList() {
    const container = document.getElementById('panel-node-list');
    if (!container) return;
    if (allNodes.length === 0) {
      container.innerHTML = '<span style="color:var(--muted);font-size:12px">无匹配节点</span>';
      return;
    }
    container.innerHTML = allNodes.map(n => {
      const selected = selectedNodes.has(n.id);
      const dotColor = n.status === 'online' ? 'var(--success)' : n.status === 'offline' ? 'var(--muted)' : 'var(--warn)';
      return `<span class="node-chip ${selected ? 'selected' : ''}" data-id="${esc(n.id)}">
        <span class="dot" style="background:${dotColor}"></span>${esc(n.name || n.id)}
      </span>`;
    }).join('');

    container.querySelectorAll('.node-chip').forEach(chip => {
      chip.addEventListener('click', () => {
        const id = chip.dataset.id;
        if (selectedNodes.has(id)) {
          selectedNodes.delete(id);
          chip.classList.remove('selected');
        } else {
          selectedNodes.add(id);
          chip.classList.add('selected');
        }
        saveSelection();
        updateSelectedCount();
        updateExecButton();
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
        if (p && p !== currentPage) {
          currentPage = p;
          loadNodes();
        }
      });
    });
  }

  function updateSelectedCount() {
    const el = document.getElementById('selected-count');
    if (el) el.textContent = selectedNodes.size === 0 ? '未选择（默认全部匹配节点）' : `已选 ${selectedNodes.size} 个节点`;
  }

  async function loadFilters() {
    try {
      const res = await api.filters();
      if (res.groups) allGroups = res.groups;
      if (res.labels) allLabels = res.labels;
      renderFilterControls();
    } catch {}
  }

  function toggleGroup(g) {
    const idx = activeGroups.indexOf(g);
    if (idx >= 0) {
      activeGroups.splice(idx, 1);
    } else {
      activeGroups.push(g);
    }
    renderGroupTags();
    renderGroupChips();
    currentPage = 1;
    loadNodes();
  }

  function renderGroupChips() {
    const container = document.getElementById('group-chips');
    if (!container) return;
    container.innerHTML = allGroups.map(g => {
      const active = activeGroups.includes(g);
      return `<span class="node-chip ${active ? 'selected' : ''}" data-group="${esc(g)}">${esc(g)}</span>`;
    }).join('');
    container.querySelectorAll('.node-chip').forEach(chip => {
      chip.addEventListener('click', () => toggleGroup(chip.dataset.group));
    });
  }

  function renderGroupTags() {
    const card = document.getElementById('group-filter-card');
    const container = document.getElementById('group-tags');
    if (!container || !card) return;
    if (activeGroups.length) {
      card.style.display = '';
      container.innerHTML = activeGroups.map(g =>
        `<span class="tag ${tagColor(g)}">${esc(g)} <span class="tag-remove" data-group="${esc(g)}" style="cursor:pointer;margin-left:2px">×</span></span>`
      ).join('');
      document.querySelectorAll('.tag-remove').forEach(el => {
        el.addEventListener('click', function() { toggleGroup(this.dataset.group); });
      });
    } else {
      card.style.display = 'none';
    }
  }

  function renderFilterControls() {
    const container = document.getElementById('filter-controls');
    if (!container) return;
    container.innerHTML = `
      <div class="filter-row">
        <label>分组（点击切换多选）</label>
        <div style="display:flex;gap:4px;flex-wrap:wrap" id="group-chips"></div>
      </div>
      <div class="filter-row">
        <label>标签</label>
        <div style="display:flex;gap:4px;flex-wrap:wrap" id="label-tags"></div>
        <div style="display:flex;gap:4px;margin-top:4px">
          <input type="text" id="label-input" class="exec-input" placeholder="key=value" style="flex:1;min-width:0">
          <button class="btn btn-ghost btn-sm" id="add-label-btn">+</button>
        </div>
      </div>
    `;
    renderGroupChips();
    renderGroupTags();
    document.getElementById('add-label-btn').addEventListener('click', addLabel);
    document.getElementById('label-input').addEventListener('keydown', e => { if (e.key === 'Enter') addLabel(); });
    renderLabelTags();
  }

  function addLabel() {
    const input = document.getElementById('label-input');
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
    const container = document.getElementById('label-tags');
    if (!container) return;
    container.innerHTML = labelInputs.map(l =>
      `<span class="tag tag-blue">${esc(l)} <span class="label-remove" data-label="${esc(l)}" style="cursor:pointer;margin-left:2px">×</span></span>`
    ).join('');
    document.querySelectorAll('.label-remove').forEach(el => {
      el.addEventListener('click', function() { removeLabel(this.dataset.label); });
    });
  }

  function hasScriptInput() {
    if (scriptInputMode === 'inline') return document.getElementById('cmd-input').value.trim() !== '';
    if (scriptInputMode === 'upload') return scriptFileContent !== '';
    if (scriptInputMode === 'url') return (document.getElementById('script-url')?.value.trim() || '') !== '';
    return false;
  }

  function updateExecButton() {
    const btn = document.getElementById('exec-btn');
    if (!btn) return;
    const count = selectedNodes.size;
    const cmd = document.getElementById('cmd-input').value.trim();
    const ready = execMode === 'script' ? hasScriptInput() : !!cmd;
    btn.disabled = !ready;
    const verb = execMode === 'script' ? '执行脚本' : '执行命令';
    btn.textContent = count === 0 ? `在全部匹配节点上${verb}` : count === 1 ? verb : `在 ${count} 个节点上${verb}`;
  }

  function switchExecMode(mode) {
    const ta = document.getElementById('cmd-input');
    if (execMode === 'command') commandContent = ta.value;
    else scriptInlineContent = ta.value;
    execMode = mode;
    const opts = document.getElementById('script-options');
    if (mode === 'command') {
      ta.value = commandContent;
      ta.placeholder = '# 输入要执行的命令，支持多行\nuptime\ndf -h\nfree -m';
      ta.style.display = '';
      opts.style.display = 'none';
    } else {
      opts.style.display = '';
      switchScriptSource(scriptInputMode);
    }
    document.getElementById('tab-command').classList.toggle('active', mode === 'command');
    document.getElementById('tab-script').classList.toggle('active', mode === 'script');
    updateExecButton();
  }

  function switchScriptSource(src) {
    const ta = document.getElementById('cmd-input');
    if (scriptInputMode === 'inline') scriptInlineContent = ta.value;
    scriptInputMode = src;
    document.getElementById('script-upload-row').style.display = src === 'upload' ? '' : 'none';
    document.getElementById('script-url-row').style.display = src === 'url' ? '' : 'none';
    if (src === 'inline') {
      ta.value = scriptInlineContent;
      ta.placeholder = '#!/bin/bash\n# 输入脚本内容\necho "hello world"';
      ta.style.display = '';
    } else {
      ta.value = '';
      ta.style.display = 'none';
    }
    updateExecButton();
  }

  function appendTerminal(html, cls) {
    const body = document.getElementById('term-body');
    if (!body) return;
    body.innerHTML = body.innerHTML.replace('<div class="line cursor-blink"></div>', '');
    const line = document.createElement('div');
    line.className = 'line';
    if (cls) line.innerHTML = `<span class="${cls}">${html}</span>`;
    else line.innerHTML = html;
    body.appendChild(line);
    body.innerHTML += '<div class="line cursor-blink"></div>';
    body.scrollTop = body.scrollHeight;
  }

  function clearTerminal() {
    const body = document.getElementById('term-body');
    if (!body) return;
    body.innerHTML = '<div class="line cursor-blink"></div>';
  }

  function renderLogDownloads() {
    const el = document.getElementById('exec-log-downloads');
    if (!el || !currentOpID) return;
    const nodes = Array.from(new Set(currentTasks.map(t => t.node_id).filter(Boolean)));
    el.innerHTML = `
      <div class="card">
        <div class="card-header"><h3>执行日志</h3><span style="font-size:12px;color:var(--muted)">按节点保存，可下载</span></div>
        <div class="card-body">
          <button class="btn btn-secondary btn-sm" id="dl-zip"><svg width="14" height="14" aria-hidden="true"><use href="#icon-download"/></svg> 下载全部日志 (zip)</button>
          <div style="margin-top:8px;display:flex;gap:6px;flex-wrap:wrap">
            ${nodes.map(n => `<button class="btn btn-ghost btn-sm" data-dl-node="${esc(n)}">${esc(n)}.log</button>`).join('')}
          </div>
        </div>
      </div>`;
    el.style.display = '';
    el.querySelector('#dl-zip').addEventListener('click', () => {
      api.executionLogArchive(currentOpID).catch(e => alert('下载失败: ' + (e.message || e)));
    });
    el.querySelectorAll('[data-dl-node]').forEach(b => {
      b.addEventListener('click', () => {
        api.executionLogDownload(currentOpID, b.dataset.dlNode).catch(e => alert('下载失败: ' + (e.message || e)));
      });
    });
  }

  function hasTargetFilter() {
    return selectedNodes.size > 0 || activeGroups.length > 0 || labelInputs.length > 0;
  }

  // 统计实际执行目标节点数：已手动选中则直接计数；
  // 否则按执行 payload 的分组/标签语义(与 /exec 服务端 SelectIntersect 一致，
  // 不含状态/搜索框等仅影响预览列表的筛选)查询总数。
  async function countExecTargetNodes() {
    if (selectedNodes.size > 0) return selectedNodes.size;
    const opts = {};
    if (activeGroups.length) opts.group = activeGroups.join(',');
    const labels = labelInputs.map(l => {
      const i = l.indexOf('=');
      return i > 0 ? l.slice(0, i) + ':' + l.slice(i + 1) : null;
    }).filter(Boolean);
    if (labels.length) opts.label = labels;
    opts.page = 1;
    opts.page_size = 1;
    const res = await api.nodes(opts);
    return res.meta?.total || 0;
  }

  async function loadShortcuts() {
    try {
      const res = await api.shortcuts();
      shortcuts = res.data || [];
    } catch { shortcuts = []; }
    renderShortcutChips();
  }

  function renderShortcutChips() {
    const c = document.getElementById('shortcut-chips');
    if (!c) return;
    if (shortcuts.length === 0) {
      c.innerHTML = '<span style="color:var(--muted);font-size:12px">暂无快捷命令,点 + 添加</span>';
      return;
    }
    c.innerHTML = shortcuts.map((s, i) => `
      <span class="shortcut-chip" draggable="true" data-index="${i}">
        <span class="shortcut-chip-label">${esc(s.name)}</span>
        <span class="shortcut-chip-actions">
          <button class="shortcut-chip-btn" data-act="edit" title="编辑">✎</button>
          <button class="shortcut-chip-btn" data-act="del" title="删除">×</button>
        </span>
      </span>`).join('');

    c.querySelectorAll('.shortcut-chip').forEach(chip => {
      const idx = parseInt(chip.dataset.index);
      chip.addEventListener('click', e => {
        if (e.target.closest('.shortcut-chip-actions')) return;
        const s = shortcuts[idx];
        switchExecMode('command');
        document.getElementById('cmd-input').value = s.command;
        updateExecButton();
      });
      chip.querySelectorAll('.shortcut-chip-btn').forEach(btn => {
        btn.addEventListener('click', e => {
          e.stopPropagation();
          const s = shortcuts[idx];
          if (btn.dataset.act === 'edit') openShortcutModal(s);
          else if (btn.dataset.act === 'del' && confirm(`删除快捷命令「${s.name}」?`)) deleteShortcut(s.id);
        });
      });
      chip.addEventListener('dragstart', e => {
        chip.classList.add('dragging');
        e.dataTransfer.setData('text/plain', String(idx));
        e.dataTransfer.effectAllowed = 'move';
      });
      chip.addEventListener('dragend', () => chip.classList.remove('dragging'));
    });

    c.addEventListener('dragover', e => {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
    });
    c.addEventListener('drop', e => {
      e.preventDefault();
      const from = parseInt(e.dataTransfer.getData('text/plain'));
      if (!Number.isInteger(from) || from < 0 || from >= shortcuts.length) return;
      const chips = c.querySelectorAll('.shortcut-chip');
      let to = shortcuts.length;
      for (let i = 0; i < chips.length; i++) {
        const rect = chips[i].getBoundingClientRect();
        if (e.clientX < rect.left + rect.width / 2) {
          to = i;
          break;
        }
      }
      if (to === from || to === from + 1) return;
      const arr = shortcuts.slice();
      const [moved] = arr.splice(from, 1);
      arr.splice(to > from ? to - 1 : to, 0, moved);
      shortcuts = arr;
      renderShortcutChips();
      api.reorderShortcuts(shortcuts.map(x => x.id)).catch(() => loadShortcuts());
    });
  }

  function openShortcutModal(s) {
    editingShortcutID = s ? s.id : null;
    document.getElementById('shortcut-modal-title').textContent = s ? '编辑快捷命令' : '新增快捷命令';
    document.getElementById('shortcut-name').value = s ? s.name : '';
    document.getElementById('shortcut-command').value = s ? s.command : '';
    document.getElementById('shortcut-error').textContent = '';
    document.getElementById('shortcut-modal').classList.add('open');
  }

  async function saveShortcut() {
    const name = document.getElementById('shortcut-name').value.trim();
    const command = document.getElementById('shortcut-command').value.trim();
    const err = document.getElementById('shortcut-error');
    if (!name || !command) { err.textContent = '名称和命令都不能为空'; return; }
    try {
      if (editingShortcutID) {
        await api.updateShortcut(editingShortcutID, { name, command });
      } else {
        await api.createShortcut({ name, command });
      }
      document.getElementById('shortcut-modal').classList.remove('open');
      loadShortcuts();
    } catch (e) { err.textContent = e.message || '保存失败'; }
  }

  async function deleteShortcut(id) {
    try {
      await api.deleteShortcut(id);
      loadShortcuts();
    } catch (e) { alert('删除失败: ' + (e.message || e)); }
  }

  function buildExecPayload() {
    const nodeIDs = Array.from(selectedNodes);
    const cmd = document.getElementById('cmd-input').value.trim();
    const formatEl = document.getElementById('format-select');
    const isAsync = document.getElementById('async-toggle')?.checked || false;
    const isSerial = document.getElementById('mode-serial')?.checked || false;
    const isDebug = document.getElementById('debug-toggle')?.checked || false;
    const retryCount = parseInt(document.getElementById('retry-count')?.value) || 3;
    const retryInterval = document.getElementById('retry-interval')?.value || '1';
    const retryMaxInterval = document.getElementById('retry-max-interval')?.value || '30';
    const noRetry = document.getElementById('no-retry')?.checked || false;
    const connectTimeout = document.getElementById('connect-timeout')?.value || '';
    const commandTimeout = document.getElementById('command-timeout')?.value || '';

    const payload = {
      force: 'true',
    };
    if (nodeIDs.length) payload.node_ids = nodeIDs;

    if (execMode === 'script') {
      payload.mode = 'script';
      if (scriptInputMode === 'inline') {
        payload.script_content = document.getElementById('cmd-input').value;
      } else if (scriptInputMode === 'upload') {
        payload.script_content = scriptFileContent;
      } else if (scriptInputMode === 'url') {
        payload.script_url = document.getElementById('script-url')?.value.trim() || '';
      }
      const sName = document.getElementById('script-name')?.value.trim();
      if (sName) payload.script_name = sName;
      else if (scriptInputMode === 'upload' && scriptFileName) payload.script_name = scriptFileName;
      const sDest = document.getElementById('script-dest')?.value.trim();
      if (sDest) payload.script_dest = sDest;
      const sArgs = document.getElementById('script-args')?.value.trim();
      if (sArgs) payload.script_args = sArgs;
      if (document.getElementById('script-keep')?.checked) payload.script_keep = true;
    } else {
      payload.command = cmd;
    }

    if (isAsync) payload.async = true;
    if (formatEl && formatEl.value !== 'simple') payload.format = formatEl.value;
    if (isSerial) payload.serial = true;
    if (isDebug) payload.debug = true;
    if (retryCount !== 3) payload.retry = retryCount;
    if (retryInterval !== '1') payload.retry_interval = retryInterval + 's';
    if (retryMaxInterval !== '30') payload.retry_max_interval = retryMaxInterval + 's';
    if (noRetry) payload.no_retry = true;
    if (connectTimeout) payload.connect_timeout = connectTimeout + 's';
    if (commandTimeout) payload.command_timeout = commandTimeout + 's';

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

  async function handleExec() {
    const cmd = document.getElementById('cmd-input').value.trim();
    if (execMode === 'script') {
      if (!hasScriptInput()) return;
    } else if (!cmd) {
      return;
    }

    const nodeIDs = Array.from(selectedNodes);
    const isAsync = document.getElementById('async-toggle')?.checked || false;

    let targetCount = 0;
    try {
      targetCount = await countExecTargetNodes();
    } catch {}

    const noFilter = !hasTargetFilter();
    if (noFilter) {
      const scope = targetCount > 0 ? `全部 ${targetCount} 个节点` : '全部匹配节点';
      if (!confirm(`⚠️ 未选择任何分组/标签，也未手动选择节点。\n命令将执行到【${scope}】！\n\n确定要继续吗？`)) return;
    } else if (targetCount > 50) {
      if (!confirm(`⚠️ 本次操作将同时在 ${targetCount} 个节点上执行，超过 50 个。\n\n确定要继续吗？`)) return;
    }

    clearTerminal();
    const dl = document.getElementById('exec-log-downloads');
    if (dl) dl.style.display = 'none';
    const modeLabel = (isAsync ? '[异步]' : '') + (execMode === 'script' ? '[脚本]' : '');
    appendTerminal(`${modeLabel}正在连接 ${nodeIDs.length === 0 ? '全部匹配' : nodeIDs.length + ' 个'} 节点…`, 'ts');

    try {
      const payload = buildExecPayload();
      const res = await api.execAdvanced(payload);

      const tasks = res.tasks || [];
      currentTaskIDs = tasks.map(t => t.id);
      currentTasks = tasks;
      currentOpID = (tasks[0] && tasks[0].record_id) || '';

      if (tasks.length === 0) {
        appendTerminal('✗ 未创建任何任务', 'err');
        return;
      }

      const isSingle = tasks.length === 1;

      if (isSingle) {
        appendTerminal(`任务已创建: ${esc(tasks[0].id)}`, 'ok');
      } else {
        tasks.forEach(t => {
          appendTerminal(`[${esc(t.node_id)}] 任务: ${esc(t.id)}`, 'out');
        });
      }
      appendTerminal(isAsync ? '提交成功，等待异步完成…' : '等待实时输出…', 'ts');

      if (wsCleanup) wsCleanup.close();
      const finished = new Set();
      wsCleanup = api.connectWebSocket(msg => {
        if (msg.type === 'task_output') {
          const t = msg.data;
          if (!t || !currentTaskIDs.includes(t.task_id)) return;
          const prefix = isSingle ? '' : `[${esc(t.node_id)}] `;
          appendTerminal(prefix + esc(t.line), t.type === 'stderr' ? 'err' : 'out');
        } else if (msg.type === 'task_update') {
          const t = msg.data;
          if (!t || !currentTaskIDs.includes(t.id) || finished.has(t.id)) return;
          finished.add(t.id);
          if (t.status === 'completed') {
            appendTerminal(isSingle ? '✓ 执行完成' : `[${esc(t.node_id)}] ✓ 完成`, 'ok');
          } else if (t.status === 'failed' || t.status === 'cancelled') {
            appendTerminal(isSingle ? `✗ ${t.status === 'cancelled' ? '已取消' : '执行失败'}` : `[${esc(t.node_id)}] ✗ ${t.status === 'cancelled' ? '已取消' : '执行失败'}`, 'err');
          }
          if (finished.size >= currentTaskIDs.length) {
            appendTerminal('— 全部任务已结束，可在任务历史中查看输出 —', 'ts');
            renderLogDownloads();
            if (wsCleanup) wsCleanup.close();
          }
        }
      });
    } catch (e) {
      appendTerminal('✗ 执行失败: ' + esc(e.message || '未知错误'), 'err');
    }
  }

  render(`
    <div class="exec-layout">
      <div class="exec-main">
        <div class="cmd-editor">
          <div class="shortcut-bar">
            <span class="shortcut-bar-title">快捷命令</span>
            <div class="shortcut-chips" id="shortcut-chips"></div>
            <button class="btn btn-ghost btn-sm" id="add-shortcut-btn" title="新增快捷命令">＋</button>
          </div>
          <div class="editor-header">
            <div class="seg">
              <button class="active" id="tab-command">命令</button>
              <button id="tab-script">脚本</button>
            </div>
            <span style="display:flex;gap:6px">
              <button class="btn btn-ghost btn-sm" id="clear-cmd-btn">清空</button>
            </span>
          </div>
          <textarea id="cmd-input" placeholder="# 输入要执行的命令，支持多行&#10;uptime&#10;df -h&#10;free -m" aria-label="命令输入框" spellcheck="false">uptime
df -h
free -m</textarea>
          <div id="script-options" style="display:none;padding:10px 12px;border-top:1px solid var(--border)">
            <div class="filter-row">
              <label>脚本来源</label>
              <div class="seg">
                <button class="active" data-script-src="inline">内联</button>
                <button data-script-src="upload">上传</button>
                <button data-script-src="url">URL</button>
              </div>
            </div>
            <div class="filter-row" id="script-upload-row" style="display:none">
              <label>脚本文件</label>
              <input type="file" id="script-file" class="exec-input">
            </div>
            <div class="filter-row" id="script-url-row" style="display:none">
              <label>脚本 URL</label>
              <input type="text" id="script-url" class="exec-input" placeholder="https://example.com/deploy.sh">
            </div>
            <div class="param-group" style="display:flex;gap:12px;flex-wrap:wrap;align-items:flex-end">
              <div class="param-row">
                <label>脚本名</label>
                <input type="text" id="script-name" class="exec-input" placeholder="自动" style="width:120px">
              </div>
              <div class="param-row">
                <label>目标目录</label>
                <input type="text" id="script-dest" class="exec-input" value="/tmp" style="width:100px">
              </div>
              <div class="param-row">
                <label>参数</label>
                <input type="text" id="script-args" class="exec-input" placeholder="--env prod" style="width:140px">
              </div>
              <label class="force-check" style="padding-bottom:6px">
                <input type="checkbox" id="script-keep"> 保留脚本
              </label>
            </div>
          </div>
        </div>

        <div class="output-terminal">
          <div class="term-header">
            <div class="dot-group">
              <span class="td td-red"></span>
              <span class="td td-yellow"></span>
              <span class="td td-green"></span>
            </div>
            <span class="term-title">输出</span>
            <span style="flex:1"></span>
            <button class="btn btn-ghost btn-sm" onclick="window.location='/history'">历史记录</button>
            <button class="btn btn-ghost btn-sm" id="clear-term-btn">清屏</button>
          </div>
          <div class="term-body" id="term-body">
            <div class="line" style="color:var(--muted)">选择节点并点击「执行」查看输出</div>
            <div class="line cursor-blink"></div>
          </div>
        </div>

        <div id="exec-log-downloads" style="display:none;margin-top:12px"></div>

        <div class="modal-overlay" id="shortcut-modal">
          <div class="modal modal-sm">
            <h3 id="shortcut-modal-title">新增快捷命令</h3>
            <div class="modal-form">
              <div class="form-row"><label>名称</label><input id="shortcut-name" placeholder="如:磁盘占用"></div>
              <div class="form-row"><label>命令</label><textarea id="shortcut-command" placeholder="df -h" style="min-height:80px;width:100%"></textarea></div>
            </div>
            <p class="error-msg" id="shortcut-error"></p>
            <div class="modal-actions">
              <button class="btn btn-secondary" id="shortcut-cancel">取消</button>
              <button class="btn btn-primary" id="shortcut-save">保存</button>
            </div>
          </div>
        </div>
      </div>

      <div class="exec-sidebar">
        <div class="card">
          <div class="card-header"><h3>筛选条件</h3></div>
          <div class="card-body" id="filter-controls"></div>
        </div>

        <div class="card" id="group-filter-card" style="${activeGroups.length ? '' : 'display:none'}">
          <div class="card-header"><h3>活跃分组</h3></div>
          <div class="card-body" id="group-tags" style="display:flex;gap:4px;flex-wrap:wrap">
            ${activeGroups.map(g => `<span class="tag ${tagColor(g)}">${esc(g)}</span>`).join('')}
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3>执行模式</h3></div>
          <div class="card-body">
            <div class="seg" style="margin-bottom:8px">
              <button class="active" id="mode-parallel" data-mode="parallel">并行</button>
              <button id="mode-serial" data-mode="serial">串行</button>
            </div>
            <label class="toggle-row">
              <input type="checkbox" id="async-toggle">
              <span class="toggle-track"><span class="toggle-thumb"></span></span>
              <span style="font-size:12px;color:var(--muted)">异步执行</span>
            </label>
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3>输出选项</h3></div>
          <div class="card-body">
            <div class="filter-row">
              <label>格式</label>
              <select id="format-select" class="exec-select">
                <option value="simple">simple</option>
                <option value="detail">detail</option>
                <option value="json">json</option>
              </select>
            </div>
            <label class="toggle-row" style="margin-top:6px">
              <input type="checkbox" id="debug-toggle">
              <span class="toggle-track"><span class="toggle-thumb"></span></span>
              <span style="font-size:12px;color:var(--muted)">调试模式</span>
            </label>
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3>超时设置</h3></div>
          <div class="card-body">
            <div class="param-group">
              <div class="param-row">
                <label>连接超时</label>
                <div style="display:flex;gap:4px;align-items:center">
                  <input type="number" id="connect-timeout" class="exec-input" value="10" min="1" style="width:50px;text-align:center">
                  <span style="font-size:11px;color:var(--muted)">s</span>
                </div>
              </div>
              <div class="param-row">
                <label>命令超时</label>
                <div style="display:flex;gap:4px;align-items:center">
                  <input type="number" id="command-timeout" class="exec-input" value="30" min="1" style="width:50px;text-align:center">
                  <span style="font-size:11px;color:var(--muted)">s</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="adv-toggle" onclick="document.getElementById('retry-options').classList.toggle('open');this.querySelector('.arrow').classList.toggle('open')">
            <span class="arrow">▶</span> 重试设置
          </div>
          <div class="adv-options open" id="retry-options">
            <div class="adv-option-group">
              <label>最大重试次数</label>
              <input type="number" id="retry-count" value="3" min="0" style="width:60px">
            </div>
            <div class="adv-option-group">
              <label>重试间隔</label>
              <div style="display:flex;gap:4px;align-items:center">
                <input type="number" id="retry-interval" value="1" min="1" style="width:50px">
                <span style="font-size:10px;color:var(--muted)">s</span>
              </div>
            </div>
            <div class="adv-option-group">
              <label>最大间隔</label>
              <div style="display:flex;gap:4px;align-items:center">
                <input type="number" id="retry-max-interval" value="30" min="1" style="width:50px">
                <span style="font-size:10px;color:var(--muted)">s</span>
              </div>
            </div>
            <div class="adv-option-group">
              <label class="force-check" style="padding-top:14px">
                <input type="checkbox" id="no-retry"> 禁用重试
              </label>
            </div>
          </div>
        </div>

        <button class="btn btn-primary" id="exec-btn" style="width:100%;justify-content:center;padding:10px" disabled>
          <svg width="16" height="16" aria-hidden="true"><use href="#icon-play"/></svg>
          选择目标节点
        </button>
      </div>
    </div>
  `, () => {
    loadNodes();
    loadFilters();
    loadShortcuts();
    updateSelectedCount();

    document.getElementById('cmd-input').addEventListener('input', updateExecButton);
    document.getElementById('exec-btn').addEventListener('click', handleExec);
    document.getElementById('clear-cmd-btn').addEventListener('click', () => {
      document.getElementById('cmd-input').value = '';
      updateExecButton();
    });
    document.getElementById('clear-term-btn').addEventListener('click', clearTerminal);

    document.querySelectorAll('.status-btn').forEach(btn => {
      btn.addEventListener('click', function() {
        document.querySelectorAll('.status-btn').forEach(b => b.classList.remove('active'));
        this.classList.add('active');
        statusFilter = this.dataset.status;
        currentPage = 1;
        selectedNodes.clear();
        saveSelection();
        loadNodes();
        updateExecButton();
      });
    });

    ['mode-parallel', 'mode-serial'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.addEventListener('click', () => {
        document.getElementById('mode-parallel').classList.toggle('active', id === 'mode-parallel');
        document.getElementById('mode-serial').classList.toggle('active', id === 'mode-serial');
      });
    });

    document.getElementById('tab-command').addEventListener('click', () => switchExecMode('command'));
    document.getElementById('tab-script').addEventListener('click', () => switchExecMode('script'));

    document.querySelectorAll('[data-script-src]').forEach(btn => {
      btn.addEventListener('click', function() {
        document.querySelectorAll('[data-script-src]').forEach(b => b.classList.remove('active'));
        this.classList.add('active');
        switchScriptSource(this.dataset.scriptSrc);
      });
    });

    const scriptFileEl = document.getElementById('script-file');
    if (scriptFileEl) scriptFileEl.addEventListener('change', function() {
      const file = this.files && this.files[0];
      if (!file) { scriptFileContent = ''; scriptFileName = ''; updateExecButton(); return; }
      scriptFileName = file.name;
      const reader = new FileReader();
      reader.onload = () => { scriptFileContent = reader.result; updateExecButton(); };
      reader.readAsText(file);
    });

    const scriptUrlEl = document.getElementById('script-url');
    if (scriptUrlEl) scriptUrlEl.addEventListener('input', updateExecButton);

    document.getElementById('panel-node-search').addEventListener('input', function() {
      searchQuery = this.value.trim();
      currentPage = 1;
      loadNodes();
    });

    document.getElementById('select-all-btn').addEventListener('click', selectAllFiltered);
    document.getElementById('clear-selection-btn').addEventListener('click', clearSelection);

    document.getElementById('add-shortcut-btn').addEventListener('click', () => openShortcutModal(null));
    document.getElementById('shortcut-save').addEventListener('click', saveShortcut);
    document.getElementById('shortcut-cancel').addEventListener('click', () => document.getElementById('shortcut-modal').classList.remove('open'));
    document.getElementById('shortcut-modal').addEventListener('click', e => {
      if (e.target === e.currentTarget) e.currentTarget.classList.remove('open');
    });
    document.getElementById('shortcut-name').addEventListener('keydown', e => { if (e.key === 'Enter') saveShortcut(); });
    document.getElementById('shortcut-command').addEventListener('keydown', e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) saveShortcut(); });

    const noRetryCb = document.getElementById('no-retry');
    if (noRetryCb) {
      noRetryCb.addEventListener('change', function() {
        document.getElementById('retry-count').disabled = this.checked;
        document.getElementById('retry-interval').disabled = this.checked;
        document.getElementById('retry-max-interval').disabled = this.checked;
      });
    }
  });
}
