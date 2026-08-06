export function renderTasks(render, navigate, user, api) {
  let tasks = [];
  let totalTasks = 0;
  let currentPage = 1;
  const pageSize = 50;
  let allNodes = [];
  let selectedNodeIDs = new Set();
  let filteredNodes = [];
  let execTimers = {};
  let wsCleanup = null;
  let currentResults = null;

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'s'; if (s<3600) return Math.floor(s/60)+'m'; return Math.floor(s/3600)+'h'; }
  function elapsed(t) { if (!t) return '0s'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'s'; return Math.floor(s/60)+'m '+s%60+'s'; }

  function statusClass(s) { return 'status-badge status-' + (s === 'completed' ? 'online' : s === 'failed' || s === 'cancelled' ? 'offline' : 'unknown'); }
  function statusDot(s) { return '<span class="status-dot '+s+'"></span>'; }

  function truncateCmd(cmd, max) {
    if (!cmd || cmd.length <= max) return esc(cmd || '');
    return '<span title="'+esc(cmd)+'">'+esc(cmd.slice(0, max))+'<span class="cmd-truncated"></span></span>';
  }

  function startTimer(taskId, startedAt) {
    stopTimer(taskId);
    const el = document.getElementById('timer-'+taskId);
    if (!el || !startedAt) return;
    execTimers[taskId] = setInterval(() => {
      el.textContent = elapsed(startedAt);
    }, 1000);
  }

  function stopTimer(taskId) {
    if (execTimers[taskId]) { clearInterval(execTimers[taskId]); delete execTimers[taskId]; }
  }

  async function loadTasks() {
    try {
      const res = await api.tasks({ page: currentPage, page_size: pageSize });
      tasks = res.data || [];
      totalTasks = (res.meta && res.meta.total) || tasks.length;
    } catch { tasks = []; totalTasks = 0; }
    renderTable();
    renderPagination();
  }

  function renderTable() {
    const list = document.getElementById('task-list');
    if (!list) return;
    if (tasks.length === 0) {
      list.innerHTML = '<tr><td colspan="4" class="empty-state">No tasks yet</td></tr>';
      return;
    }
    list.innerHTML = tasks.map(t => {
      const isRunning = t.status === 'running';
      const isQueued = t.status === 'queued';
      return '<tr>' +
        '<td><a href="/tasks/'+esc(t.id)+'" style="font-family:\'SF Mono\',Monaco,monospace;font-size:12px">'+esc(t.node_id)+'</a></td>' +
        '<td class="cmd-cell">'+truncateCmd(t.command, 60)+'</td>' +
        '<td><span class="'+statusClass(t.status)+'">'+statusDot(t.status)+esc(t.status)+
          (isRunning ? ' <span class="running-timer" id="timer-'+esc(t.id)+'">'+elapsed(t.started_at)+'</span>' : '')+
          (t.exit_code !== null && t.exit_code !== undefined ? ' ('+t.exit_code+')' : '')+
        '</span></td>' +
        '<td style="font-size:12px;color:var(--text-muted)">'+timeAgo(t.created_at)+'</td>' +
      '</tr>';
    }).join('');

    tasks.forEach(t => {
      if (t.status === 'running' && t.started_at) startTimer(t.id, t.started_at);
    });
  }

  function renderPagination() {
    const el = document.getElementById('pagination');
    if (!el) return;
    const totalPages = Math.ceil(totalTasks / pageSize);
    if (totalPages <= 1) { el.innerHTML = ''; return; }
    let html = '<div class="pagination">';
    html += '<button data-page="'+(currentPage-1)+'"'+(currentPage===1?' disabled':'')+'>‹</button>';
    for (let i = Math.max(1, currentPage-2); i <= Math.min(totalPages, currentPage+2); i++) {
      html += '<button data-page="'+i+'"'+(i===currentPage?' class="active"':'')+'>'+i+'</button>';
    }
    html += '<button data-page="'+(currentPage+1)+'"'+(currentPage===totalPages?' disabled':'')+'>›</button>';
    html += '<span class="page-info">'+currentPage+'/'+totalPages+'</span>';
    html += '</div>';
    el.innerHTML = html;
    el.querySelectorAll('button:not(:disabled)').forEach(btn => {
      btn.addEventListener('click', () => {
        currentPage = parseInt(btn.dataset.page);
        loadTasks();
      });
    });
  }

  function updateTaskInList(updated) {
    const idx = tasks.findIndex(t => t.id === updated.id);
    if (idx >= 0) {
      tasks[idx] = updated;
    } else {
      tasks.unshift(updated);
      totalTasks++;
    }
    renderTable();
    if (currentResults && currentResults.some(t => t.id === updated.id)) {
      renderResults(currentResults.map(t => t.id === updated.id ? updated : t));
    }
  }

  async function loadAllNodes() {
    try {
      const res = await api.nodes({ page_size: 500 });
      allNodes = res.data || [];
    } catch { allNodes = []; }
  }

  async function loadFilters() {
    try {
      const res = await api.filters();
      const f = res.data || res;
      const groups = f.groups || [];
      const statuses = f.statuses || ['online', 'offline', 'unknown'];
      const groupSel = document.getElementById('exec-filter-group');
      if (groupSel) {
        groupSel.innerHTML = '<option value="">All Groups</option>'+groups.map(g => '<option value="'+esc(g)+'">'+esc(g)+'</option>').join('');
      }
      const statusSel = document.getElementById('exec-filter-status');
      if (statusSel) {
        statusSel.innerHTML = '<option value="">All Status</option>'+statuses.map(s => '<option value="'+esc(s)+'">'+esc(s)+'</option>').join('');
      }
    } catch {}
  }

  function applyFilters() {
    const group = document.getElementById('exec-filter-group').value;
    const status = document.getElementById('exec-filter-status').value;
    const search = document.getElementById('exec-filter-search').value.toLowerCase();

    filteredNodes = allNodes.filter(n => {
      try { const g = JSON.parse(n.groups || '[]'); if (group && !g.includes(group)) return false; } catch {}
      if (status && n.status !== status) return false;
      if (search && !(n.name || '').toLowerCase().includes(search) && !(n.address || '').includes(search) && !(n.id || '').includes(search)) return false;
      return true;
    });
    renderNodeList();
    updateSelectionInfo();
  }

  function renderNodeList() {
    const list = document.getElementById('exec-node-list');
    if (!list) return;
    if (filteredNodes.length === 0) {
      list.innerHTML = '<div style="padding:12px;text-align:center;color:var(--text-muted);font-size:12px">No nodes match filters</div>';
      return;
    }
    list.innerHTML = filteredNodes.map(n => {
      let groups = [];
      try { groups = JSON.parse(n.groups || '[]'); } catch {}
      let labels = [];
      try { const l = JSON.parse(n.labels || '{}'); labels = Object.entries(l).slice(0, 2).map(([k,v]) => k+'='+v); } catch {}
      const tags = [...groups, ...labels];
      return '<label class="node-list-item">' +
        '<input type="checkbox" value="'+esc(n.id)+'"'+(selectedNodeIDs.has(n.id)?' checked':'')+'>' +
        '<span>'+esc(n.name || n.id)+'</span>' +
        '<span class="node-addr">'+esc(n.user||'')+'@'+esc(n.address||'')+'</span>' +
        (tags.length ? '<span class="node-tags">'+tags.map(t => '<span class="node-tag">'+esc(t)+'</span>').join('')+'</span>' : '') +
      '</label>';
    }).join('');

    list.querySelectorAll('input[type="checkbox"]').forEach(cb => {
      cb.addEventListener('change', () => {
        if (cb.checked) selectedNodeIDs.add(cb.value);
        else selectedNodeIDs.delete(cb.value);
        updateSelectionInfo();
      });
    });

    list.querySelectorAll('.node-list-item').forEach(label => {
      label.addEventListener('click', (e) => {
        if (e.target.tagName === 'INPUT') return;
        const cb = label.querySelector('input[type="checkbox"]');
        cb.checked = !cb.checked;
        cb.dispatchEvent(new Event('change'));
      });
    });
  }

  function updateSelectionInfo() {
    const countEl = document.getElementById('exec-selected-count');
    const totalEl = document.getElementById('exec-total-count');
    const summaryEl = document.getElementById('exec-summary-count');
    const submitEl = document.getElementById('exec-submit');
    if (countEl) countEl.textContent = selectedNodeIDs.size;
    if (totalEl) totalEl.textContent = allNodes.length;
    if (summaryEl) summaryEl.textContent = selectedNodeIDs.size;
    if (submitEl) submitEl.disabled = selectedNodeIDs.size === 0;
  }

  function showExecModal() {
    document.getElementById('exec-error').textContent = '';
    document.getElementById('exec-modal').classList.add('open');
    applyFilters();
  }

  function hideExecModal() {
    document.getElementById('exec-modal').classList.remove('open');
  }

  function getExecRequest() {
    const isScript = document.querySelector('.modal-tab.active').dataset.tab === 'script';
    const cmd = document.getElementById('exec-cmd').value.trim();
    const scriptContent = document.getElementById('exec-script-content').value.trim();
    const scriptName = document.getElementById('exec-script-name').value.trim();
    const scriptArgs = document.getElementById('exec-script-args').value.trim();
    const force = document.getElementById('exec-force').checked;

    if (!isScript && !cmd) { throw new Error('Command is required'); }
    if (isScript && !scriptContent) { throw new Error('Script content is required'); }

    const data = {
      node_ids: Array.from(selectedNodeIDs),
      force: force ? 'true' : undefined,
    };
    if (isScript) {
      data.script_content = scriptContent;
      if (scriptName) data.script_name = scriptName;
      if (scriptArgs) data.script_args = scriptArgs;
    } else {
      data.command = cmd;
    }
    return data;
  }

  async function submitExec() {
    document.getElementById('exec-error').textContent = '';
    const submitBtn = document.getElementById('exec-submit');
    submitBtn.disabled = true;
    submitBtn.textContent = 'Executing...';
    try {
      const data = getExecRequest();
      const res = await api.execAdvanced(data);
      hideExecModal();
      currentPage = 1;
      loadTasks();
      if (res.tasks && res.tasks.length > 0) {
        currentResults = res.tasks;
        renderResults(res.tasks);
      }
    } catch (e) {
      document.getElementById('exec-error').textContent = e.message;
    }
    submitBtn.disabled = false;
    submitBtn.textContent = 'Execute';
  }

  function renderResults(resultTasks) {
    const panel = document.getElementById('results-panel');
    if (!panel || !resultTasks || resultTasks.length === 0) { if (panel) panel.innerHTML = ''; return; }
    const done = resultTasks.filter(t => t.status === 'completed' || t.status === 'failed' || t.status === 'cancelled').length;
    panel.innerHTML =
      '<div class="results-panel">' +
        '<div class="results-header">' +
          '<span>Results — <strong>'+done+'/'+resultTasks.length+'</strong> done</span>' +
          '<button class="close-btn" id="results-close">×</button>' +
        '</div>' +
        resultTasks.map(t => {
          const isRunning = t.status === 'running';
          return '<div class="result-card">' +
            '<div class="result-meta">' +
              statusDot(t.status) +
              '<span class="node-name">'+esc(t.node_id)+'</span>' +
              '<span class="'+statusClass(t.status)+'">'+esc(t.status)+(isRunning ? ' <span class="running-timer">'+elapsed(t.started_at)+'</span>' : '')+(t.exit_code !== null && t.exit_code !== undefined ? ' ('+t.exit_code+')' : '')+'</span>' +
            '</div>' +
            (t.output ? '<div class="result-output">'+esc(t.output)+'</div>' : '') +
            (t.error ? '<div class="result-output" style="color:var(--danger)">'+esc(t.error)+'</div>' : '') +
          '</div>';
        }).join('') +
      '</div>';

    const closeBtn = document.getElementById('results-close');
    if (closeBtn) closeBtn.addEventListener('click', () => { currentResults = null; panel.innerHTML = ''; });
  }

  const canExec = ['admin', 'operator'].includes(user.role);

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
      <div class="page-header">
        <h2>Tasks</h2>
        ${canExec ? '<button class="btn btn-primary" id="exec-btn"><span style="font-size:16px">▶</span> Execute</button>' : ''}
      </div>
      <div class="card">
        <table>
          <thead><tr><th>Node</th><th>Command</th><th>Status</th><th>Time</th></tr></thead>
          <tbody id="task-list"><tr><td colspan="4" class="loading">Loading...</td></tr></tbody>
        </table>
      </div>
      <div id="pagination"></div>
      <div id="results-panel"></div>
    </div>

    <div class="modal-overlay" id="exec-modal">
      <div class="modal modal-wide" style="max-height:90vh;overflow-y:auto">
        <div class="modal-tabs">
          <button class="modal-tab active" data-tab="command">Command</button>
          <button class="modal-tab" data-tab="script">Script</button>
        </div>

        <div class="modal-tab-content active" id="tab-command">
          <label class="form-label">Command</label>
          <textarea class="exec-textarea" id="exec-cmd" placeholder="Enter command... e.g. uptime, df -h, systemctl status nginx"></textarea>
        </div>

        <div class="modal-tab-content" id="tab-script">
          <input class="exec-script-name" id="exec-script-name" placeholder="Script name (e.g. deploy.sh)">
          <label class="form-label">Script Content</label>
          <textarea class="exec-textarea" id="exec-script-content" placeholder="#!/bin/bash\nset -e\necho 'hello'" style="min-height:120px"></textarea>
          <input class="exec-script-name" id="exec-script-args" placeholder="Arguments (e.g. --env prod --version 2.1)" style="margin-top:8px">
        </div>

        <label class="form-label" style="margin-top:16px">Target Nodes</label>
        <div class="node-selector-bar">
          <select id="exec-filter-group"><option value="">All Groups</option></select>
          <select id="exec-filter-status"><option value="">All Status</option></select>
          <input id="exec-filter-search" placeholder="Search name/address...">
        </div>
        <div class="node-list" id="exec-node-list"><div style="padding:12px;text-align:center;color:var(--text-muted);font-size:12px">Loading nodes...</div></div>
        <div class="node-select-info">
          <span><span class="count" id="exec-selected-count">0</span> / <span id="exec-total-count">0</span> selected</span>
          <div>
            <button id="exec-select-all">Select All</button>
            <button id="exec-clear-all">Clear</button>
          </div>
        </div>

        <div class="adv-toggle" id="adv-toggle">
          <span class="arrow">▶</span> Advanced Options
        </div>
        <div class="adv-options" id="adv-options">
          <div class="adv-option-group">
            <label>Mode</label>
            <select id="exec-mode">
              <option value="parallel">Parallel</option>
              <option value="serial">Serial</option>
            </select>
          </div>
          <div class="adv-option-group">
            <label>Connect Timeout</label>
            <input id="exec-connect-timeout" value="10s">
          </div>
          <div class="adv-option-group">
            <label>Command Timeout</label>
            <input id="exec-command-timeout" value="30s">
          </div>
          <div class="adv-option-group">
            <label>Retries</label>
            <input id="exec-retry" value="3" type="number" min="0">
          </div>
          <div class="adv-option-group">
            <label class="force-check">
              <input type="checkbox" id="exec-force"> Force (skip conflict check)
            </label>
          </div>
        </div>

        <div class="exec-summary">
          <span class="info">Target <strong id="exec-summary-count">0</strong> node(s)</span>
          <div style="display:flex;gap:8px">
            <button class="btn btn-secondary" id="exec-cancel">Cancel</button>
            <button class="btn btn-primary" id="exec-submit" disabled>Execute</button>
          </div>
        </div>
        <p class="error-msg" id="exec-error" style="margin-top:8px"></p>
      </div>
    </div>
  `, () => {
    document.getElementById('logout-btn').addEventListener('click', () => { localStorage.removeItem('token'); localStorage.removeItem('user'); navigate('/login'); });

    wsCleanup = api.connectWebSocket((msg) => {
      if (msg.type === 'task_update' && msg.data) {
        updateTaskInList(msg.data);
      }
    });

    if (canExec) {
      document.getElementById('exec-btn').addEventListener('click', showExecModal);

      // Tab switching
      document.querySelectorAll('.modal-tab').forEach(tab => {
        tab.addEventListener('click', () => {
          document.querySelectorAll('.modal-tab').forEach(t => t.classList.remove('active'));
          document.querySelectorAll('.modal-tab-content').forEach(c => c.classList.remove('active'));
          tab.classList.add('active');
          document.getElementById('tab-'+tab.dataset.tab).classList.add('active');
        });
      });

      // Node filter events
      document.getElementById('exec-filter-group').addEventListener('change', applyFilters);
      document.getElementById('exec-filter-status').addEventListener('change', applyFilters);
      document.getElementById('exec-filter-search').addEventListener('input', applyFilters);

      // Select all / clear
      document.getElementById('exec-select-all').addEventListener('click', () => {
        filteredNodes.forEach(n => selectedNodeIDs.add(n.id));
        renderNodeList();
        updateSelectionInfo();
      });
      document.getElementById('exec-clear-all').addEventListener('click', () => {
        selectedNodeIDs.clear();
        renderNodeList();
        updateSelectionInfo();
      });

      // Advanced options toggle
      document.getElementById('adv-toggle').addEventListener('click', () => {
        const opts = document.getElementById('adv-options');
        const arrow = document.querySelector('#adv-toggle .arrow');
        opts.classList.toggle('open');
        arrow.classList.toggle('open');
      });

      // Submit
      document.getElementById('exec-submit').addEventListener('click', submitExec);

      // Cancel / close
      document.getElementById('exec-cancel').addEventListener('click', hideExecModal);
      document.getElementById('exec-modal').addEventListener('click', (e) => {
        if (e.target === e.currentTarget) hideExecModal();
      });
      window.escHandler = (e) => { if (e.key === 'Escape') hideExecModal(); };
      document.addEventListener('keydown', window.escHandler);

      // Load initial data for the modal
      loadAllNodes();
      loadFilters();
    }

    return () => {
      Object.values(execTimers).forEach(clearInterval);
      if (wsCleanup) wsCleanup.close();
      document.removeEventListener('keydown', escHandler);
    };
  });
}
