export function renderPlaybooks(render, navigate, user, api, shell) {
  let state = {
    playbooks: [],
    filteredPlaybooks: [],
    query: '',
    selectedCategory: '',
    categories: [],
    runsPage: 1,
    runsPageSize: 20,
    runsTotal: 0,
  };

  let ws = null;

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  async function loadSettingsPath() {
    try {
      const res = await api.playbookSettingsPath();
      if (res.value) document.getElementById('playbook-path').value = res.value;
    } catch {}
  }

  async function loadAll() {
    await loadSettingsPath();
    try {
      const res = await api.playbooks();
      state.playbooks = res.data || [];
    } catch { state.playbooks = []; }
    extractCategories();
    applyFilters();
  }

  function extractCategories() {
    const cats = new Set();
    cats.add('');
    for (const pb of state.playbooks) {
      if (pb.category) cats.add(pb.category);
    }
    state.categories = Array.from(cats).sort();
  }

  function applyFilters() {
    state.filteredPlaybooks = state.playbooks.filter(pb => {
      if (state.selectedCategory && pb.category !== state.selectedCategory) return false;
      if (state.query) {
        const q = state.query.toLowerCase();
        const name = (pb.name || '').toLowerCase();
        const desc = (pb.description || '').toLowerCase();
        if (!name.includes(q) && !desc.includes(q)) return false;
      }
      return true;
    });
    renderTable();
    renderPanel();
  }

  function renderPanel() {
    const counts = {};
    for (const pb of state.playbooks) {
      const c = pb.category || '';
      counts[c] = (counts[c] || 0) + 1;
    }
    const html = [
      `<li class="panel-item ${state.selectedCategory === '' ? 'active' : ''}" data-category="" role="option" aria-selected="${state.selectedCategory === ''}">
        <span class="dot" style="background:var(--accent)"></span>
        <span>全部</span>
        <span class="count">${state.playbooks.length}</span>
      </li>`,
      ...state.categories.filter(c => c !== '').map(c => `
        <li class="panel-item ${state.selectedCategory === c ? 'active' : ''}" data-category="${esc(c)}" role="option" aria-selected="${state.selectedCategory === c}">
          <span class="dot" style="background:var(--muted)"></span>
          <span>${esc(c)}</span>
          <span class="count">${counts[c] || 0}</span>
        </li>`)
    ].join('');

    shell.setPanelContent(`<ul style="list-style:none;padding:0">${html}</ul>`);

    document.querySelectorAll('.panel-item[data-category]').forEach(el => {
      el.addEventListener('click', () => {
        state.selectedCategory = el.dataset.category;
        applyFilters();
      });
    });
  }

  function renderTable() {
    const list = document.getElementById('playbook-list');
    if (state.filteredPlaybooks.length === 0) {
      list.innerHTML = '<tr><td colspan="5" class="empty-state">无匹配剧本</td></tr>';
    } else {
      list.innerHTML = state.filteredPlaybooks.map(pb =>
        `<tr class="playbook-row" data-id="${esc(pb.id)}">
          <td class="cell-ellipsis" title="${esc(pb.name)}">${esc(pb.name)}${pb.file_exists === false ? ' <span class="missing-badge">缺失</span>' : ''}</td>
          <td>${pb.category ? `<span class="tag">${esc(pb.category)}</span>` : '<span class="cell-muted">-</span>'}</td>
          <td class="cell-ellipsis" title="${esc(pb.description || '')}">${esc(pb.description || '')}</td>
          <td class="cell-ellipsis cell-muted" title="${esc((pb.task_names || []).join('、'))}">${esc((pb.task_names || []).join('、'))}</td>
          <td class="action-cell">
            <button class="btn btn-ghost btn-sm run-playbook-btn" data-id="${esc(pb.id)}" ${pb.file_exists === false ? 'disabled' : ''}>运行</button>
            <button class="btn btn-ghost btn-sm edit-playbook-btn" data-id="${esc(pb.id)}" ${pb.file_exists === false ? 'disabled' : ''} title="二次编辑剧本">编辑</button>
            <button class="btn btn-ghost btn-sm download-playbook-btn" data-id="${esc(pb.id)}" ${pb.file_exists === false ? 'disabled' : ''} title="下载 playbook 文件">下载</button>
          </td>
        </tr>`
      ).join('');
    }
    document.querySelectorAll('.run-playbook-btn').forEach(btn => {
      btn.addEventListener('click', (e) => { e.stopPropagation(); showRunModal(btn.dataset.id); });
    });
    document.querySelectorAll('.edit-playbook-btn').forEach(btn => {
      btn.addEventListener('click', (e) => { e.stopPropagation(); showEditModal(btn.dataset.id); });
    });
    document.querySelectorAll('.download-playbook-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        e.stopPropagation();
        try { await api.playbookDownload(btn.dataset.id); } catch (err) { alert('下载失败: ' + err.message); }
      });
    });
    document.querySelectorAll('.playbook-row').forEach(row => {
      row.addEventListener('click', () => showPlaybookDetail(row.dataset.id));
    });
  }

  // Create playbook wizard state (top-level so renderTable can reference showEditModal)
  let cpState = {
    step: 1,
    totalSteps: 5,
    vars: [],
    tasks: [],
  };
  let cpTaskCounter = 0;

  function resetCpModal() {
    cpState = { step: 1, totalSteps: 5, vars: [], tasks: [] };
    cpTaskCounter = 0;
    document.getElementById('cp-name').value = '';
    document.getElementById('cp-category').value = '';
    document.getElementById('cp-desc').value = '';
    document.getElementById('cp-version').value = '1.0';
    document.getElementById('cp-mode').value = '';
    document.getElementById('cp-groups').value = '';
    document.getElementById('cp-tags').value = '';
    document.getElementById('cp-skip-tags').value = '';
    document.getElementById('cp-vars-list').innerHTML = '';
    document.getElementById('cp-skip-vars').checked = false;
    document.getElementById('cp-tasks-list').innerHTML = '<p class="empty-state" style="font-size:var(--fs-sm);padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>';
    document.getElementById('cp-error').textContent = '';
    showCpStep(1);
  }

  async function showEditModal(id) {
    let data;
    try {
      data = await api.playbookEdit(id);
    } catch (e) { alert('加载剧本失败: ' + e.message); console.error(e); return; }

    try {
      document.getElementById('cp-title').textContent = '✏️ 编辑剧本: ' + (data.name || id);
      resetCpModal();
      document.getElementById('cp-name').value = data.name || '';
      document.getElementById('cp-category').value = data.category || '';
      document.getElementById('cp-desc').value = data.description || '';
      document.getElementById('cp-version').value = data.version || '1.0';
      document.getElementById('cp-mode').value = data.execution_mode || '';
      document.getElementById('cp-groups').value = (data.default_groups || []).join(',');
      document.getElementById('cp-tags').value = (data.default_tags || []).join(',');
      document.getElementById('cp-skip-tags').value = (data.default_skip_tags || []).join(',');
      document.getElementById('cp-skip-vars').checked = !data.vars || Object.keys(data.vars).length === 0;

      cpState.vars = Object.entries(data.vars || {}).map(([k, v]) => ({ key: k, value: String(v) }));
      cpState.tasks = (data.tasks || []).map(t => ({ name: t.name, action: t.action, args: t.args || {} }));
      cpState.preTasks = data.pre_tasks || [];
      cpState.postTasks = data.post_tasks || [];
      cpTaskCounter = cpState.tasks.length;
      renderCpVars();
      renderCpTasks();

      document.getElementById('cp-title').textContent = '✏️ 编辑剧本: ' + (data.name || '');
      document.getElementById('create-playbook-modal').classList.add('open');
    } catch (e) {
      alert('打开编辑窗口失败: ' + e.message);
      console.error(e);
    }
  }

  function renderCpVars() {
    const list = document.getElementById('cp-vars-list');
    list.innerHTML = '';
    cpState.vars.forEach((v, idx) => {
      const row = document.createElement('div');
      row.className = 'form-row';
      row.style.cssText = 'display:flex;gap:8px;align-items:end';
      const keyInp = document.createElement('input');
      keyInp.className = 'cp-var-key';
      keyInp.placeholder = '变量名';
      keyInp.style.width = '100%';
      keyInp.value = v.key;
      keyInp.addEventListener('input', () => { cpState.vars[idx].key = keyInp.value; });
      const valInp = document.createElement('input');
      valInp.className = 'cp-var-value';
      valInp.placeholder = '值';
      valInp.style.width = '100%';
      valInp.value = v.value;
      valInp.addEventListener('input', () => { cpState.vars[idx].value = valInp.value; });
      const delBtn = document.createElement('button');
      delBtn.textContent = '删除';
      delBtn.style.cssText = 'background:none;border:1px solid var(--danger);color:var(--danger);border-radius:var(--radius);cursor:pointer;padding:4px 8px;font-size:var(--fs-xs)';
      delBtn.addEventListener('click', () => {
        const removeIdx = Array.from(list.children).indexOf(row);
        cpState.vars.splice(removeIdx, 1);
        row.remove();
      });
      const keyWrap = document.createElement('div');
      keyWrap.style.flex = '1';
      keyWrap.appendChild(keyInp);
      const valWrap = document.createElement('div');
      valWrap.style.flex = '1';
      valWrap.appendChild(valInp);
      row.append(keyWrap, valWrap, delBtn);
      list.appendChild(row);
    });
  }

  function showCpStep(n) {
    cpState.step = n;
    document.querySelectorAll('.create-pb-page').forEach(el => el.style.display = 'none');
    document.querySelector(`.create-pb-page[data-page="${n}"]`).style.display = 'block';
    document.querySelectorAll('.step-dot').forEach(el => el.classList.toggle('active', parseInt(el.dataset.step) <= n));
    document.getElementById('cp-prev-btn').style.display = n > 1 ? '' : 'none';
    document.getElementById('cp-next-btn').style.display = n < cpState.totalSteps ? '' : 'none';
    document.getElementById('cp-save-btn').style.display = n === cpState.totalSteps ? '' : 'none';
    if (n === cpState.totalSteps) buildCpSummary();
    document.getElementById('cp-error').textContent = '';
  }

  function getActionArgs(action) {
    const templates = {
      command: { cmd: '<命令内容>' },
      script: { script: '<脚本路径>', dest: '/tmp/', args: '' },
      upload: { src: '<本地路径>', dest: '<远程路径>', overwrite: true },
      download: { src: '<远程路径>', dest: '<本地路径>', subdir: true },
      include: { playbook: '<剧本路径>' },
    };
    return JSON.parse(JSON.stringify(templates[action] || {}));
  }

  function renderCpTasks() {
    const list = document.getElementById('cp-tasks-list');
    list.innerHTML = '';
    if (cpState.tasks.length === 0) {
      list.innerHTML = '<p class="empty-state" style="font-size:var(--fs-sm);padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>';
      return;
    }
    const actions = ['command', 'script', 'upload', 'download', 'include'];
    cpState.tasks.forEach((t, i) => {
      const card = document.createElement('div');
      card.style.cssText = 'border:1px solid var(--border);border-radius:var(--radius);margin-bottom:8px;padding:8px';

      const head = document.createElement('div');
      head.style.cssText = 'display:flex;gap:8px;align-items:center;margin-bottom:6px';
      const nameInput = document.createElement('input');
      nameInput.className = 'cp-task-name';
      nameInput.placeholder = '任务名称';
      nameInput.style.flex = '1';
      nameInput.value = t.name;
      nameInput.addEventListener('input', () => { cpState.tasks[i].name = nameInput.value; });
      const actionSel = document.createElement('select');
      actionSel.style.width = '130px';
      for (const a of actions) {
        const opt = document.createElement('option');
        opt.value = a;
        opt.textContent = a;
        if (t.action === a) opt.selected = true;
        actionSel.appendChild(opt);
      }
      actionSel.addEventListener('change', () => { cpState.tasks[i].action = actionSel.value; });
      const removeBtn = document.createElement('button');
      removeBtn.textContent = '删除';
      removeBtn.style.cssText = 'background:none;border:1px solid var(--danger);color:var(--danger);border-radius:var(--radius);cursor:pointer;padding:2px 8px;font-size:var(--fs-xs);white-space:nowrap';
      removeBtn.addEventListener('click', () => { cpState.tasks.splice(i, 1); renderCpTasks(); });
      head.append(nameInput, actionSel, removeBtn);

      const argsBox = document.createElement('div');
      argsBox.className = 'cp-task-args';
      argsBox.style.cssText = 'display:flex;flex-direction:column;gap:4px';
      renderArgRowsInto(argsBox, i, t.args);

      const addArgBtn = document.createElement('button');
      addArgBtn.textContent = '+ 添加参数';
      addArgBtn.style.cssText = 'background:none;border:1px dashed var(--border);color:var(--muted);border-radius:var(--radius);cursor:pointer;padding:2px 8px;font-size:var(--fs-xs);margin-top:6px';
      addArgBtn.addEventListener('click', () => { cpState.tasks[i].args[''] = ''; renderCpTasks(); });

      card.append(head, argsBox, addArgBtn);
      list.appendChild(card);
    });
  }

  function renderArgRowsInto(box, idx, args) {
    for (const [k, v] of Object.entries(args)) {
      const row = document.createElement('div');
      row.style.cssText = 'display:flex;gap:6px;align-items:center';
      const keyInp = document.createElement('input');
      keyInp.className = 'cp-arg-key';
      keyInp.placeholder = '参数名';
      keyInp.style.width = '140px';
      keyInp.value = k;
      keyInp.addEventListener('change', () => {
        if (keyInp.value === k) return;
        const task = cpState.tasks[idx];
        renameArg(task.args, k, keyInp.value);
        renderCpTasks();
      });
      const valInp = document.createElement('input');
      valInp.className = 'cp-arg-value';
      valInp.placeholder = '值';
      valInp.style.flex = '1';
      valInp.value = typeof v === 'string' ? v : JSON.stringify(v);
      valInp.addEventListener('input', () => { cpState.tasks[idx].args[k] = valInp.value; });
      const delBtn = document.createElement('button');
      delBtn.textContent = '×';
      delBtn.title = '删除参数';
      delBtn.style.cssText = 'background:none;border:none;color:var(--danger);cursor:pointer;font-size:var(--fs-sm)';
      delBtn.addEventListener('click', () => { delete cpState.tasks[idx].args[k]; renderCpTasks(); });
      row.append(keyInp, valInp, delBtn);
      box.appendChild(row);
    }
  }

  function renameArg(args, oldKey, newKey) {
    if (oldKey === newKey) return;
    if (newKey === '') { delete args[oldKey]; return; }
    if (oldKey in args) { args[newKey] = args[oldKey]; delete args[oldKey]; }
  }

  // 字符串值推断为 bool/number（保留类型语义，如 overwrite: true）
  function coerceArgValues(args) {
    const out = {};
    for (const [k, v] of Object.entries(args)) {
      let val = v;
      if (typeof val === 'string') {
        const t = val.trim();
        if (t === 'true') val = true;
        else if (t === 'false') val = false;
        else if (t !== '' && !isNaN(Number(t))) val = Number(t);
      }
      if (k.trim() !== '') out[k.trim()] = val;
    }
    return out;
  }

  function buildCpSummary() {
    const name = document.getElementById('cp-name').value.trim();
    const desc = document.getElementById('cp-desc').value.trim();
    const category = document.getElementById('cp-category').value.trim();
    const version = document.getElementById('cp-version').value.trim() || '1.0';
    const mode = document.getElementById('cp-mode').value || 'fail_continue';

    const html = `
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px">
        <div><strong>名称:</strong> ${esc(name)}</div>
        <div><strong>版本:</strong> ${esc(version)}</div>
        <div><strong>描述:</strong> ${esc(desc || '-')}</div>
        <div><strong>分类:</strong> ${esc(category || '-')}</div>
        <div><strong>执行模式:</strong> ${mode}</div>
      </div>
    `;
    document.getElementById('cp-summary').innerHTML = html;

    // Build preview YAML (for visual only — the actual YAML is generated server-side)
    const lines = [`name: ${name}`];
    if (category) lines.push(`category: ${category}`);
    if (desc) lines.push(`description: ${desc}`);
    lines.push(`version: "${version}"`, 'hosts: []');
    if (mode) lines.push(`execution_mode: ${mode}`);
    lines.push('pre_tasks: []', 'tasks:');
    for (const t of cpState.tasks) {
      lines.push(`  - name: ${t.name}`, `    action: ${t.action}`, '    args:');
      for (const [k, v] of Object.entries(t.args)) {
        const val = typeof v === 'string' ? v : JSON.stringify(v);
        lines.push(`      ${k}: ${val}`);
      }
    }
    lines.push('post_tasks: []');
    document.getElementById('cp-preview').textContent = lines.join('\n');
  }

  async function loadRunStagingFiles() {
    const listEl = document.getElementById('run-staging-files');
    const dirEl = document.getElementById('run-staging-dir');
    if (!listEl) return;
    listEl.innerHTML = '加载中…';
    try {
      const [fRes, dRes] = await Promise.all([api.staging.files(), api.staging.disk()]);
      const files = fRes.data || [];
      if (dRes && dRes.staging_dir) dirEl.textContent = `中转站目录: ${dRes.staging_dir}`;
      if (files.length === 0) {
        listEl.innerHTML = '<span style="color:var(--muted)">暂无文件，可先在「文件」页上传</span>';
        return;
      }
      listEl.innerHTML = files.map(f => `
        <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;padding:2px 0;border-bottom:1px dashed var(--border)">
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(f.name)} <span style="color:var(--muted)">(${f.size} B)</span></span>
          <button class="staging-copy-path" data-name="${esc(f.name)}" title="复制完整路径" style="background:none;border:1px solid var(--border);color:var(--text-muted);border-radius:var(--radius);cursor:pointer;padding:0 8px;font-size:var(--fs-xs)">复制路径</button>
        </div>`).join('');
      document.querySelectorAll('.staging-copy-path').forEach(btn => {
        btn.addEventListener('click', async () => {
          const dir = (document.getElementById('run-staging-dir').textContent || '').replace('中转站目录: ', '').trim();
          const full = dir ? `${dir.replace(/[\\/]$/, '')}/${btn.dataset.name}` : btn.dataset.name;
          try {
            await navigator.clipboard.writeText(full);
            btn.textContent = '已复制';
            setTimeout(() => { btn.textContent = '复制路径'; }, 1500);
          } catch { prompt('复制路径:', full); }
        });
      });
    } catch {
      listEl.innerHTML = '<span style="color:var(--danger)">中转站文件加载失败</span>';
    }
  }

  let runSel = { nodes: new Set(), groups: new Set(), tags: new Set() };
  let runNodes = [];
  let runGroupCounts = {};

  function showRunModal(id) {
    const pb = state.playbooks.find(p => p.id === id);
    runSel = { nodes: new Set(), groups: new Set(), tags: new Set() };
    document.getElementById('run-playbook-id').value = id;
    document.getElementById('run-playbook-name-display').textContent = pb ? pb.name : id;
    document.getElementById('run-playbook-target').value = '';
    document.getElementById('run-playbook-vars').value = '';
    document.getElementById('run-playbook-error').textContent = '';
    document.getElementById('run-node-dropdown').style.display = 'none';
    const warnEl = document.getElementById('run-playbook-warnings');
    if (warnEl) { warnEl.style.display = 'none'; warnEl.textContent = ''; }
    document.getElementById('run-playbook-modal').classList.add('open');
    loadRunTargetData(id);
    loadRunStagingFiles();
  }

  async function loadRunTargetData(pbId) {
    try {
      const [nodesRes, filtersRes] = await Promise.all([api.nodes(), api.filters()]);
      runNodes = nodesRes.data || [];
      const counts = {};
      for (const n of runNodes) {
        for (const g of (n.groups || [])) counts[g] = (counts[g] || 0) + 1;
      }
      runGroupCounts = counts;
      renderRunGroups((filtersRes.groups || []).sort());
      renderRunNodeSelected();
    } catch { renderRunGroups([]); }

    if (pbId) {
      api.playbookEdit(pbId).then(edit => {
        renderRunTags((edit.tags || []).slice().sort());
      }).catch(() => { renderRunTags([]); });
    } else {
      renderRunTags([]);
    }
  }

  function filterRunNodes(q) {
    q = (q || '').toLowerCase();
    if (!q) return runNodes;
    return runNodes.filter(n => {
      return (n.id || '').toLowerCase().includes(q)
        || (n.name || '').toLowerCase().includes(q)
        || (n.address || '').toLowerCase().includes(q);
    });
  }

  function renderRunNodeDropdown() {
    const q = document.getElementById('run-playbook-target').value;
    const dd = document.getElementById('run-node-dropdown');
    const matched = filterRunNodes(q);
    if (matched.length === 0) {
      dd.innerHTML = '<div style="padding:8px;color:var(--muted);font-size:var(--fs-xs)">无匹配节点</div>';
    } else {
      dd.innerHTML = matched.map(n => {
        const checked = runSel.nodes.has(n.id);
        const namePart = n.name && n.name !== n.id ? ` <span style="color:var(--muted)">(${esc(n.name)})</span>` : '';
        return `<label style="display:flex;align-items:center;gap:6px;padding:5px 8px;cursor:pointer;font-size:var(--fs-xs)">
          <input type="checkbox" data-node="${esc(n.id)}" ${checked ? 'checked' : ''} style="width:13px;height:13px;flex:0 0 auto;cursor:pointer;accent-color:var(--accent)">
          <span>${esc(n.id)}</span>${namePart}
          <span style="margin-left:auto;color:var(--muted)">${esc(n.address || '')}</span>
        </label>`;
      }).join('');
    }
    dd.querySelectorAll('input[data-node]').forEach(inp => {
      inp.addEventListener('click', (e) => {
        const nodeId = inp.dataset.node;
        if (inp.checked) runSel.nodes.add(nodeId); else runSel.nodes.delete(nodeId);
        renderRunNodeSelected();
      });
    });
  }

  function renderRunNodeSelected() {
    const box = document.getElementById('run-node-selected');
    if (runSel.nodes.size === 0) {
      box.innerHTML = '';
      return;
    }
    box.innerHTML = Array.from(runSel.nodes).map(id => `
      <span class="tag" style="display:inline-flex;align-items:center;gap:4px;padding:2px 8px">
        ${esc(id)}<button class="run-node-remove" data-node="${esc(id)}" style="background:none;border:none;color:inherit;cursor:pointer;font-size:var(--fs-xs);padding:0">&times;</button>
      </span>`).join('');
    box.querySelectorAll('.run-node-remove').forEach(btn => {
      btn.addEventListener('click', () => {
        runSel.nodes.delete(btn.dataset.node);
        renderRunNodeSelected();
        renderRunNodeDropdown();
      });
    });
  }

  function renderRunGroups(groups) {
    const grid = document.getElementById('run-group-grid');
    if (!groups.length) {
      grid.innerHTML = '<span style="color:var(--muted);font-size:var(--fs-xs)">暂无分组</span>';
      return;
    }
    grid.innerHTML = groups.map(g => `
      <label style="display:inline-flex;align-items:center;gap:4px;border:1px solid var(--border);border-radius:var(--radius);padding:3px 8px;cursor:pointer;font-size:var(--fs-xs)">
        <input type="checkbox" data-group="${esc(g)}" style="width:13px;height:13px;flex:0 0 auto;cursor:pointer;accent-color:var(--accent)">
        <span>${esc(g)}</span>
        <span style="color:var(--muted)">(${runGroupCounts[g] || 0})</span>
      </label>`).join('');
    grid.querySelectorAll('input[data-group]').forEach(inp => {
      inp.addEventListener('change', () => {
        if (inp.checked) runSel.groups.add(inp.dataset.group); else runSel.groups.delete(inp.dataset.group);
      });
    });
  }

  function renderRunTags(tags) {
    const grid = document.getElementById('run-tag-grid');
    if (!tags.length) {
      grid.innerHTML = '<span style="color:var(--muted);font-size:var(--fs-xs)">该剧本无标签任务</span>';
      return;
    }
    grid.innerHTML = tags.map(t => `
      <label style="display:inline-flex;align-items:center;gap:4px;border:1px solid var(--border);border-radius:var(--radius);padding:3px 8px;cursor:pointer;font-size:var(--fs-xs)">
        <input type="checkbox" data-tag="${esc(t)}" style="width:13px;height:13px;flex:0 0 auto;cursor:pointer;accent-color:var(--accent)">
        <span>${esc(t)}</span>
      </label>`).join('');
    grid.querySelectorAll('input[data-tag]').forEach(inp => {
      inp.addEventListener('change', () => {
        if (inp.checked) runSel.tags.add(inp.dataset.tag); else runSel.tags.delete(inp.dataset.tag);
      });
    });
  }

  async function showPlaybookDetail(id) {
    const modal = document.getElementById('playbook-detail-modal');
    const meta = document.getElementById('detail-meta');
    const tasks = document.getElementById('detail-tasks');
    const yaml = document.getElementById('detail-yaml');
    const nameEl = document.getElementById('detail-name');

    modal.dataset.playbookId = id;
    nameEl.textContent = '加载中…';
    meta.innerHTML = '';
    tasks.innerHTML = '';
    yaml.textContent = '';
    modal.classList.add('open');

    try {
      const pb = await api.playbookGet(id);
      nameEl.textContent = pb.name;
      meta.innerHTML = `
        <span>分类: <strong>${pb.category ? esc(pb.category) : '-'}</strong></span>
        <span>描述: ${esc(pb.description || '-')}</span>
        <span>文件: ${esc(pb.file_path)}</span>
        <span>状态: ${pb.file_exists ? '<span style="color:var(--success)">存在</span>' : '<span style="color:var(--danger)">缺失</span>'}</span>
      `;
      if (pb.task_names && pb.task_names.length > 0) {
        tasks.innerHTML = `<h4 style="font-size:var(--fs-sm);color:var(--muted);margin-bottom:8px">任务列表 (${pb.task_names.length})</h4>
          <div style="display:flex;flex-wrap:wrap;gap:6px">${pb.task_names.map(t => `<span class="tag">${esc(t)}</span>`).join('')}</div>`;
      } else {
        tasks.innerHTML = '<p style="color:var(--muted);font-size:var(--fs-sm)">无任务</p>';
      }
      if (pb.file_exists) {
        yaml.textContent = '加载 YAML…';
        api.playbookFile(id).then(res => {
          yaml.textContent = res.content || '(空文件)';
        }).catch(() => {
          yaml.textContent = `文件路径: ${pb.file_path}\n\n(YAML 加载失败)`;
        });
      } else {
        yaml.textContent = '文件不存在';
      }
    } catch {
      nameEl.textContent = '加载失败';
      meta.innerHTML = '<span style="color:var(--danger)">无法获取剧本详情</span>';
    }
  }

  // 事件委托绑定在容器上(仅一次):run 更新触发的 loadRuns() 会整体重绘表格,
  // 若监听器绑在按钮上,重绘发生在 mousedown→mouseup 之间时按钮被替换,
  // 浏览器不再派发 click 事件 → 表现为「View 没有响应」。
  // 改用 pointerdown 记录意图 + pointerup 执行(pointer 事件不受重绘影响)。
  function delegateRunActions(list) {
    if (list.dataset.runDelegated) return;
    list.dataset.runDelegated = '1';

    list.addEventListener('pointerdown', (e) => {
      if (e.button !== 0) return;
      const btn = e.target.closest('.view-run-btn, .cancel-run-btn');
      list._pendingRunAction = btn
        ? { type: btn.classList.contains('cancel-run-btn') ? 'cancel' : 'view', id: btn.dataset.id }
        : null;
    });

    list.addEventListener('pointerup', (e) => {
      if (e.button !== 0) return;
      const pending = list._pendingRunAction;
      list._pendingRunAction = null;
      if (!pending) return;
      if (!e.target.closest('.view-run-btn, .cancel-run-btn')) return;

      if (pending.type === 'cancel') {
        if (!confirm('确定取消该剧本运行？')) return;
        api.cancelPlaybookRun(pending.id)
          .then(loadRuns)
          .catch(err => alert('Cancel failed: ' + err.message));
        return;
      }

      const id = pending.id;
      history.replaceState(null, '', '/playbooks?run=' + encodeURIComponent(id));
      api.playbookRun(id).then(run => {
        const detail = document.getElementById('run-detail');
        if (detail) detail.dataset.runId = id;
        showRunDetail(run);
        const card = document.getElementById('run-detail-card');
        if (card) card.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      }).catch(err => showRunDetailError(err));
    });
  }

  function renderRuns(runs) {
    const list = document.getElementById('playbook-runs-list');
    if (!list) return;
    if (!runs || runs.length === 0) {
      list.innerHTML = '<tr><td colspan="5" class="empty-state">暂无运行记录</td></tr>';
    } else {
      list.innerHTML = runs.map(r => {
        const nodes = r.target_nodes || [];
        const nodesText = nodes.length > 3
          ? nodes.slice(0, 3).join(', ') + ` 等 ${nodes.length} 个节点`
          : nodes.join(', ');
        return `<tr>
        <td class="cell-ellipsis" title="${esc(r.playbook_name)}">${esc(r.playbook_name)}</td>
        <td class="cell-ellipsis" title="${esc(nodes.join(', '))}">${esc(nodesText)}</td>
        <td><span class="status-badge status-${esc(r.status)}">${esc(r.status)}</span></td>
        <td class="cell-muted">${r.created_at ? new Date(r.created_at).toLocaleString('zh-CN', { hour12: false }) : ''}</td>
        <td class="action-cell">
          <button class="btn btn-ghost btn-sm view-run-btn" data-id="${r.id}">查看</button>
          ${r.status === 'running' || r.status === 'pending' ? `<button class="btn btn-ghost btn-sm cancel-run-btn" data-id="${r.id}" style="color:var(--danger)">取消</button>` : ''}
        </td>
      </tr>`;
      }).join('');
    }
    delegateRunActions(list);
  }

  function showRunDetailError(err) {
    const detail = document.getElementById('run-detail');
    if (!detail) return;
    detail.innerHTML = `<p class="empty-state" style="color:var(--danger)">加载运行详情失败: ${esc(err && err.message ? err.message : err)}</p>`;
    console.error('load playbook run detail failed:', err);
  }

  function showRunDetail(run) {
    const detail = document.getElementById('run-detail');
    const nodes = run.target_nodes || [];
    const steps = (run.results || []).map(r => `<tr>
      <td>${esc(r.task_name)}</td>
      <td class="cell-mono">${esc(r.node_id)}</td>
      <td>${esc(r.action || '')}</td>
      <td><span class="status-badge status-${esc(r.status)}">${esc(r.status)}</span></td>
      <td>${r.exit_code !== undefined ? r.exit_code : ''}</td>
      <td class="cell-output">${esc(r.output || '')}</td>
    </tr>`).join('');

    detail.innerHTML = `
      <div class="run-meta">
        <span class="run-meta-item"><span class="rm-label">剧本</span><strong title="${esc(run.playbook_name)}">${esc(run.playbook_name)}</strong></span>
        <span class="run-meta-item"><span class="rm-label">状态</span><span class="status-badge status-${esc(run.status)}">${esc(run.status)}</span></span>
        <span class="run-meta-item"><span class="rm-label">目标节点</span><strong class="cell-ellipsis" style="max-width:420px" title="${esc(nodes.join(', '))}">${esc(nodes.join(', ') || '—')}</strong></span>
        <span class="run-meta-item"><span class="rm-label">开始时间</span><span class="cell-muted">${run.created_at ? new Date(run.created_at).toLocaleString('zh-CN', { hour12: false }) : ''}</span></span>
        ${run.error ? `<span class="run-meta-item" style="color:var(--danger)">错误：${esc(run.error)}</span>` : ''}
      </div>
      <div style="max-height:320px;overflow:auto;border:1px solid var(--border);border-radius:var(--radius)">
        <table class="run-steps">
          <colgroup><col style="width:20%"><col style="width:14%"><col style="width:12%"><col style="width:10%"><col style="width:7%"><col style="width:37%"></colgroup>
          <thead><tr><th>任务</th><th>节点</th><th>动作</th><th>状态</th><th>退出码</th><th>输出</th></tr></thead>
          <tbody>${steps || '<tr><td colspan="6" class="empty-state">暂无步骤结果</td></tr>'}</tbody>
        </table>
      </div>
    `;
  }

  async function loadRuns() {
    try {
      const res = await api.playbookRuns({ page: state.runsPage, page_size: state.runsPageSize });
      state.runsTotal = res.meta?.total || 0;
      renderRuns(res.data || []);
    } catch { state.runsTotal = 0; renderRuns([]); }
    renderRunsPagination();
  }

  function runsTotalPages() {
    return Math.max(1, Math.ceil(state.runsTotal / state.runsPageSize));
  }

  function renderRunsPagination() {
    const info = document.getElementById('runs-page-info');
    if (!info) return;
    if (state.runsPage > runsTotalPages()) state.runsPage = runsTotalPages();
    info.textContent = `共 ${state.runsTotal} 条 · 第 ${state.runsPage}/${runsTotalPages()} 页`;
    const prev = document.getElementById('runs-prev-btn');
    const next = document.getElementById('runs-next-btn');
    if (prev) prev.disabled = state.runsPage <= 1;
    if (next) next.disabled = state.runsPage >= runsTotalPages();
  }

  function setupWebSocket() {
    ws = api.connectWebSocket(msg => {
      if (msg.type === 'playbook_run_update') {
        loadRuns();
        const detail = document.getElementById('run-detail');
        if (msg.data && detail && msg.data.id === detail.dataset.runId) {
          showRunDetail(msg.data);
        }
      }
    });
  }

  render(`
    <div class="section-card" style="margin-bottom:16px">
      <div class="panel-head">
        <div style="flex:1;min-width:0">
          <h3 class="panel-title">剧本库</h3>
          <div class="panel-desc">Library Path 指向存放 .yaml 剧本的目录，刷新后同步到列表</div>
        </div>
      </div>
      <div class="panel-body" style="padding:16px 24px">
        <div class="path-bar">
          <label for="playbook-path">Library Path</label>
          <div class="path-group">
            <input id="playbook-path" placeholder="/path/to/playbooks" spellcheck="false">
            <button class="btn btn-secondary btn-sm" id="refresh-playbooks-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg> 刷新</button>
            <button class="btn btn-secondary btn-sm" id="upload-playbook-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-upload"/></svg> 上传 Playbook</button>
          </div>
          <input type="file" id="upload-playbook-file" accept=".yaml,.yml" style="display:none">
        </div>
      </div>
    </div>

    <div class="filter-bar">
      <div class="input" style="position:relative;padding-left:32px;width:240px">
        <svg width="14" height="14" aria-hidden="true" style="position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--muted)"><use href="#icon-search"/></svg>
        <input type="text" id="playbook-search" placeholder="搜索剧本名称 / 描述…" style="border:none;background:transparent;outline:none;color:var(--fg);width:100%;font:var(--fs-sm)/1.5 var(--font-body)">
      </div>
      <div class="spacer"></div>
      <button class="btn btn-primary btn-sm" id="add-playbook-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 新建</button>
    </div>

    <div class="section-card" style="margin-top:16px">
      <div class="panel-head">
        <div style="flex:1;min-width:0">
          <h3 class="panel-title">剧本列表</h3>
          <div class="panel-desc">点击行查看详情与 YAML 源文件</div>
        </div>
      </div>
      <div class="table-scroll">
        <table class="data-table playbook-table">
          <thead><tr><th>名称</th><th>分类</th><th>描述</th><th>任务</th><th></th></tr></thead>
          <tbody id="playbook-list"><tr><td colspan="5" class="loading">加载中…</td></tr></tbody>
        </table>
      </div>
    </div>

    <div class="section-card">
      <div class="panel-head">
        <div style="flex:1;min-width:0">
          <h3 class="panel-title">运行历史</h3>
          <div class="panel-desc">每次剧本执行的记录，点击「查看」看分步结果</div>
        </div>
      </div>
      <div class="table-scroll">
        <table class="data-table">
          <thead><tr><th>剧本</th><th>目标节点</th><th>状态</th><th>开始时间</th><th></th></tr></thead>
          <tbody id="playbook-runs-list"><tr><td colspan="5" class="loading">加载中…</td></tr></tbody>
        </table>
      </div>
      <div class="panel-foot" id="runs-pagination" style="justify-content:flex-end">
        <span class="field-hint" id="runs-page-info">共 0 条 · 第 1/1 页</span>
        <span style="flex:1"></span>
        <button class="btn btn-ghost btn-sm" id="runs-prev-btn" disabled>◀ 上一页</button>
        <button class="btn btn-ghost btn-sm" id="runs-next-btn" disabled>下一页 ▶</button>
      </div>
    </div>

    <div class="section-card" id="run-detail-card">
      <div class="panel-head">
        <div style="flex:1;min-width:0">
          <h3 class="panel-title">运行详情</h3>
          <div class="panel-desc">选中运行记录后展示分步执行结果</div>
        </div>
      </div>
      <div class="panel-body" id="run-detail" data-run-id="" style="padding:16px 24px"><p class="empty-state">选择一个运行记录查看详情</p></div>
    </div>

    <div class="modal-overlay" id="run-playbook-modal">
      <div class="modal modal-sm">
        <h3>Run Playbook: <span id="run-playbook-name-display"></span></h3>
        <div class="modal-form">
          <input type="hidden" id="run-playbook-id">
          <div class="form-row">
            <label>Target Nodes</label>
            <div style="position:relative">
              <input id="run-playbook-target" placeholder="输入关键字过滤节点…" autocomplete="off">
              <div id="run-node-dropdown" style="display:none;position:absolute;top:100%;left:0;right:0;z-index:50;background:var(--bg);border:1px solid var(--border);border-radius:var(--radius);max-height:200px;overflow-y:auto;margin-top:2px;box-shadow:0 4px 12px rgba(0,0,0,.15)"></div>
            </div>
            <div id="run-node-selected" style="display:flex;flex-wrap:wrap;gap:4px;margin-top:6px"></div>
          </div>
          <div class="form-row">
            <label>Groups <span style="font-weight:normal;color:var(--muted);font-size:var(--fs-xs)">(与节点同时选择时以节点为准)</span></label>
            <div id="run-group-grid" style="display:flex;flex-wrap:wrap;gap:6px;max-height:110px;overflow-y:auto"></div>
          </div>
          <div class="form-row">
            <label>Tags <span style="font-weight:normal;color:var(--muted);font-size:var(--fs-xs)">(任务执行标签，可选)</span></label>
            <div id="run-tag-grid" style="display:flex;flex-wrap:wrap;gap:6px;max-height:110px;overflow-y:auto"></div>
          </div>
          <div class="form-row"><label>Extra Vars (optional)</label><input id="run-playbook-vars" placeholder="key=value, version=2.0"></div>
        </div>
        <p class="error-msg" id="run-playbook-error"></p>
        <p class="error-msg" id="run-playbook-warnings" style="display:none"></p>
        <div class="staging-browser" style="margin-top:8px;border:1px solid var(--border);border-radius:var(--radius);padding:8px">
          <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:6px">
            <span style="font-size:var(--fs-xs);color:var(--muted)">中转站文件（upload/script 引用时可选用）</span>
            <button class="btn btn-secondary btn-sm" id="run-staging-refresh" style="padding:1px 8px">刷新</button>
          </div>
          <div id="run-staging-files" style="max-height:160px;overflow-y:auto;font-size:var(--fs-xs);color:var(--text-muted)">加载中…</div>
          <div id="run-staging-dir" style="margin-top:6px;font-size:var(--fs-xs);color:var(--muted);word-break:break-all"></div>
        </div>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="run-playbook-cancel">Cancel</button>
          <button class="btn btn-primary" id="run-playbook-submit">Execute</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="playbook-detail-modal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3 id="detail-name"></h3>
          <button class="btn btn-ghost btn-sm" id="detail-close-btn" style="margin-left:auto;background:none;border:none;color:var(--muted);cursor:pointer;font-size:var(--fs-xl)">&times;</button>
        </div>
        <div class="modal-body" id="detail-body" style="max-height:70vh;overflow-y:auto">
          <div id="detail-meta" style="display:flex;gap:16px;flex-wrap:wrap;font-size:var(--fs-sm);margin-bottom:12px;padding-bottom:12px;border-bottom:1px solid var(--border)"></div>
          <div id="detail-tasks" style="margin-bottom:12px"></div>
          <div style="margin-top:12px">
            <h4 style="font-size:var(--fs-sm);color:var(--muted);margin-bottom:8px">YAML 源文件</h4>
            <pre id="detail-yaml" style="background:var(--code-bg);border:1px solid var(--border);border-radius:var(--radius);padding:12px;font:12px/1.6 var(--font-mono);overflow-x:auto;white-space:pre;max-height:300px"></pre>
          </div>
        </div>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="detail-cancel-btn">关闭</button>
          <button class="btn btn-primary" id="detail-run-btn">运行</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="create-playbook-modal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3 id="cp-title">📝 创建剧本</h3>
          <button class="btn btn-ghost btn-sm" id="create-pb-close-btn" style="margin-left:auto;background:none;border:none;color:var(--muted);cursor:pointer;font-size:var(--fs-xl)">&times;</button>
        </div>
        <div class="modal-body" id="create-pb-body" style="max-height:70vh;overflow-y:auto">
          <!-- Step indicator -->
          <div style="display:flex;gap:8px;margin-bottom:16px" id="create-pb-steps">
            <span class="step-dot active" data-step="1">1</span><span style="color:var(--muted);font-size:var(--fs-xs)">基本信息</span>
            <span style="color:var(--muted);padding:0 4px">→</span>
            <span class="step-dot" data-step="2">2</span><span style="color:var(--muted);font-size:var(--fs-xs)">变量</span>
            <span style="color:var(--muted);padding:0 4px">→</span>
            <span class="step-dot" data-step="3">3</span><span style="color:var(--muted);font-size:var(--fs-xs)">执行配置</span>
            <span style="color:var(--muted);padding:0 4px">→</span>
            <span class="step-dot" data-step="4">4</span><span style="color:var(--muted);font-size:var(--fs-xs)">任务</span>
            <span style="color:var(--muted);padding:0 4px">→</span>
            <span class="step-dot" data-step="5">5</span><span style="color:var(--muted);font-size:var(--fs-xs)">确认</span>
          </div>

          <!-- Step 1: Basic Info -->
          <div class="create-pb-page" data-page="1">
            <div class="form-row"><label>剧本名称 *</label><input id="cp-name" placeholder="my-playbook" style="width:100%"></div>
            <div class="form-row"><label>分类</label><input id="cp-category" placeholder="web, db, cache…（可选）" style="width:100%"></div>
            <div class="form-row"><label>描述</label><textarea id="cp-desc" placeholder="可选" style="width:100%;resize:vertical" rows="2"></textarea></div>
            <div class="form-row"><label>版本</label><input id="cp-version" value="1.0" style="width:100%"></div>
          </div>

          <!-- Step 2: Variables -->
          <div class="create-pb-page" data-page="2" style="display:none">
            <p style="font-size:var(--fs-sm);color:var(--muted);margin-bottom:12px">添加变量（可选）</p>
            <div id="cp-vars-list"></div>
            <button class="btn btn-secondary btn-sm" id="cp-add-var"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 添加变量</button>
            <div style="margin-top:8px"><label class="checkbox-label"><input type="checkbox" id="cp-skip-vars"> 跳过（不添加变量）</label></div>
          </div>

          <!-- Step 3: Execution Mode + Default Config -->
          <div class="create-pb-page" data-page="3" style="display:none">
            <div class="form-row">
              <label>执行模式</label>
              <select id="cp-mode" style="width:100%">
                <option value="">fail_continue（失败继续）</option>
                <option value="pipeline">pipeline（失败终止）</option>
              </select>
            </div>
            <h4 style="font-size:var(--fs-sm);color:var(--muted);margin:16px 0 8px">默认配置（可选）</h4>
            <div class="form-row"><label>目标分组</label><input id="cp-groups" placeholder="web, db (逗号分隔)" style="width:100%"></div>
            <div class="form-row"><label>执行标签</label><input id="cp-tags" placeholder="tag1, tag2 (逗号分隔)" style="width:100%"></div>
            <div class="form-row"><label>跳过标签</label><input id="cp-skip-tags" placeholder="skip-me (逗号分隔)" style="width:100%"></div>
          </div>

          <!-- Step 4: Tasks -->
          <div class="create-pb-page" data-page="4" style="display:none">
            <p style="font-size:var(--fs-sm);color:var(--muted);margin-bottom:12px">添加任务项</p>
            <div class="form-row" style="display:flex;gap:8px;align-items:end">
              <div style="flex:1">
                <label>任务类型</label>
                <select id="cp-task-action" style="width:100%">
                  <option value="command">command — 执行 Shell 命令</option>
                  <option value="script">script — 执行脚本文件</option>
                  <option value="upload">upload — 上传文件到节点</option>
                  <option value="download">download — 从节点下载文件</option>
                  <option value="include">include — 包含其他剧本</option>
                </select>
              </div>
              <button class="btn btn-primary btn-sm" id="cp-add-task" style="white-space:nowrap">+ 添加</button>
            </div>
            <div id="cp-tasks-list" style="margin-top:12px">
              <p class="empty-state" style="font-size:var(--fs-sm);padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>
            </div>
          </div>

          <!-- Step 5: Confirm -->
          <div class="create-pb-page" data-page="5" style="display:none">
            <div id="cp-summary" style="font-size:var(--fs-sm);margin-bottom:12px"></div>
            <h4 style="font-size:var(--fs-sm);color:var(--muted);margin-bottom:8px">YAML 预览</h4>
            <pre id="cp-preview" style="background:var(--code-bg);border:1px solid var(--border);border-radius:var(--radius);padding:12px;font:12px/1.6 var(--font-mono);overflow-x:auto;white-space:pre;max-height:300px"></pre>
          </div>
        </div>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="cp-prev-btn" style="display:none">上一步</button>
          <button class="btn btn-primary" id="cp-next-btn">下一步</button>
          <button class="btn btn-primary" id="cp-save-btn" style="display:none">保存剧本</button>
        </div>
        <p class="error-msg" id="cp-error"></p>
      </div>
    </div>
  `, () => {
    setupWebSocket();

    document.getElementById('runs-prev-btn').addEventListener('click', () => {
      if (state.runsPage > 1) { state.runsPage--; loadRuns(); }
    });
    document.getElementById('runs-next-btn').addEventListener('click', () => {
      if (state.runsPage < runsTotalPages()) { state.runsPage++; loadRuns(); }
    });

    let searchDebounceTimer;
    document.getElementById('playbook-search').addEventListener('input', (e) => {
      clearTimeout(searchDebounceTimer);
      searchDebounceTimer = setTimeout(() => {
        state.query = e.target.value.trim();
        applyFilters();
      }, 100);
    });

    document.getElementById('refresh-playbooks-btn').addEventListener('click', async () => {
      const path = document.getElementById('playbook-path').value.trim();
      if (!path) { alert('Library path is required'); return; }
      const btn = document.getElementById('refresh-playbooks-btn');
      btn.classList.add('loading');
      btn.disabled = true;
      try {
        const res = await api.refreshPlaybooks(path);
        if (res.errors && res.errors.length > 0) {
          alert('Sync completed with errors:\n' + res.errors.join('\n'));
        }
        loadAll();
      } catch (e) { alert('Refresh failed: ' + e.message); }
      btn.classList.remove('loading');
      btn.disabled = false;
    });

    document.getElementById('run-playbook-cancel').addEventListener('click', () => {
      document.getElementById('run-playbook-modal').classList.remove('open');
    });
    document.getElementById('run-playbook-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('run-playbook-modal').classList.remove('open');
    });
    document.getElementById('run-playbook-submit').addEventListener('click', async () => {
      const id = document.getElementById('run-playbook-id').value;
      const varsStr = document.getElementById('run-playbook-vars').value.trim();
      if (runSel.nodes.size === 0 && runSel.groups.size === 0) {
        document.getElementById('run-playbook-error').textContent = '请选择至少一个目标节点或分组';
        return;
      }
      const body = {};
      if (runSel.nodes.size > 0) body.target_nodes = Array.from(runSel.nodes);
      if (runSel.groups.size > 0) body.groups = Array.from(runSel.groups);
      if (runSel.tags.size > 0) body.tags = Array.from(runSel.tags).join(',');
      if (varsStr) {
        const extraVars = {};
        varsStr.split(',').forEach(pair => {
          const [k, ...vs] = pair.split('=');
          if (k && vs.length) extraVars[k.trim()] = vs.join('=').trim();
        });
        if (Object.keys(extraVars).length) body.extra_vars = extraVars;
      }
      try {
        const res = await api.runPlaybook(id, body);
        const warnings = (res && res.warnings) || [];
        if (warnings.length > 0) {
          const warnEl = document.getElementById('run-playbook-warnings');
          warnEl.style.display = 'block';
          warnEl.textContent = '⚠ 引用文件缺失（可先上传到中转站再运行）:\n' + warnings.join('\n');
          return;
        }
        document.getElementById('run-playbook-modal').classList.remove('open');
        loadRuns();
      } catch (e) { document.getElementById('run-playbook-error').textContent = e.message; }
    });

    // Target node fuzzy dropdown
    const targetInput = document.getElementById('run-playbook-target');
    targetInput.addEventListener('input', () => {
      renderRunNodeDropdown();
      document.getElementById('run-node-dropdown').style.display = 'block';
    });
    targetInput.addEventListener('focus', () => {
      renderRunNodeDropdown();
      document.getElementById('run-node-dropdown').style.display = 'block';
    });
    document.getElementById('run-node-dropdown').addEventListener('mousedown', (e) => e.preventDefault());
    document.addEventListener('click', (e) => {
      const dd = document.getElementById('run-node-dropdown');
      if (dd && !e.target.closest('#run-playbook-target') && !e.target.closest('#run-node-dropdown')) {
        dd.style.display = 'none';
      }
    });

    document.getElementById('run-staging-refresh')?.addEventListener('click', loadRunStagingFiles);
    document.getElementById('upload-playbook-btn').addEventListener('click', () => {
      document.getElementById('upload-playbook-file').click();
    });
    document.getElementById('upload-playbook-file').addEventListener('change', async (e) => {
      const file = e.target.files[0];
      e.target.value = '';
      if (!file) return;
      const btn = document.getElementById('upload-playbook-btn');
      btn.textContent = '上传中…';
      btn.disabled = true;
      try {
        await api.playbookUpload(file);
        loadAll();
      } catch (err) { alert('上传失败: ' + err.message); }
      btn.textContent = '上传 Playbook';
      btn.disabled = false;
    });

    document.getElementById('detail-close-btn').addEventListener('click', () => {
      document.getElementById('playbook-detail-modal').classList.remove('open');
    });
    document.getElementById('detail-cancel-btn').addEventListener('click', () => {
      document.getElementById('playbook-detail-modal').classList.remove('open');
    });
    document.getElementById('playbook-detail-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('playbook-detail-modal').classList.remove('open');
    });
    document.getElementById('detail-run-btn').addEventListener('click', () => {
      const id = document.getElementById('playbook-detail-modal').dataset.playbookId;
      document.getElementById('playbook-detail-modal').classList.remove('open');
      showRunModal(id);
    });

    document.getElementById('add-playbook-btn').addEventListener('click', () => {
      resetCpModal();
      document.getElementById('cp-title').textContent = '📝 创建剧本';
      document.getElementById('create-playbook-modal').classList.add('open');
    });

    document.getElementById('create-pb-close-btn').addEventListener('click', () => {
      document.getElementById('create-playbook-modal').classList.remove('open');
    });
    document.getElementById('create-playbook-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('create-playbook-modal').classList.remove('open');
    });

    document.getElementById('cp-prev-btn').addEventListener('click', () => {
      if (cpState.step > 1) showCpStep(cpState.step - 1);
    });

    document.getElementById('cp-next-btn').addEventListener('click', () => {
      const err = document.getElementById('cp-error');
      if (cpState.step === 1) {
        if (!document.getElementById('cp-name').value.trim()) {
          err.textContent = '剧本名称不能为空';
          return;
        }
      }
      if (cpState.step < cpState.totalSteps) showCpStep(cpState.step + 1);
    });

    // Variables
    document.getElementById('cp-add-var').addEventListener('click', () => {
      cpState.vars.push({ key: '', value: '' });
      renderCpVars();
    });

    // Tasks
    document.getElementById('cp-add-task').addEventListener('click', () => {
      const action = document.getElementById('cp-task-action').value;
      cpTaskCounter++;
      const task = { name: `任务 ${cpTaskCounter}`, action, args: getActionArgs(action) };
      cpState.tasks.push(task);
      renderCpTasks();
    });

    // Save
    document.getElementById('cp-save-btn').addEventListener('click', async () => {
      const err = document.getElementById('cp-error');
      const name = document.getElementById('cp-name').value.trim();
      if (!name) { err.textContent = '剧本名称不能为空'; return; }

      // Collect vars
      const vars = {};
      if (!document.getElementById('cp-skip-vars').checked) {
        for (const v of cpState.vars) {
          if (v.key.trim()) vars[v.key.trim()] = v.value;
        }
      }

      const data = {
        name,
        description: document.getElementById('cp-desc').value.trim() || undefined,
        category: document.getElementById('cp-category').value.trim() || undefined,
        version: document.getElementById('cp-version').value.trim() || '1.0',
        execution_mode: document.getElementById('cp-mode').value || undefined,
        vars: Object.keys(vars).length > 0 ? vars : undefined,
        default_groups: document.getElementById('cp-groups').value.trim() ? document.getElementById('cp-groups').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        default_tags: document.getElementById('cp-tags').value.trim() ? document.getElementById('cp-tags').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        default_skip_tags: document.getElementById('cp-skip-tags').value.trim() ? document.getElementById('cp-skip-tags').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        tasks: cpState.tasks.map(t => ({ name: t.name, action: t.action, args: coerceArgValues(t.args) })),
        pre_tasks: cpState.preTasks,
        post_tasks: cpState.postTasks,
      };

      try {
        document.getElementById('cp-save-btn').textContent = '保存中…';
        document.getElementById('cp-save-btn').disabled = true;
        await api.createPlaybookTemplate(data);
        document.getElementById('create-playbook-modal').classList.remove('open');
        loadAll();
        loadRuns();
      } catch (e) {
        err.textContent = e.message;
      } finally {
        document.getElementById('cp-save-btn').textContent = '保存剧本';
        document.getElementById('cp-save-btn').disabled = false;
      }
    });

    loadAll();
    loadRuns();

    const runId = new URLSearchParams(location.search).get('run');
    if (runId) {
      api.playbookRun(runId).then(run => {
        document.getElementById('run-detail').dataset.runId = runId;
        showRunDetail(run);
      }).catch(err => showRunDetailError(err));
    }
  });
}
