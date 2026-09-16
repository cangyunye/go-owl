export function renderPlaybooks(render, navigate, user, api, shell) {
  const PB_VIEW_KEY = 'owl-pb-view';
  const PB_LIB_KEY = 'owl-pb-lib';

  let state = {
    playbooks: [],
    filteredPlaybooks: [],
    query: '',
    selectedCategory: '',
    categories: [],
    view: localStorage.getItem(PB_VIEW_KEY) === 'grid' ? 'grid' : 'table',
    libExpanded: localStorage.getItem(PB_LIB_KEY) === '1',
    selectedId: null,
    runs: [],
    runsPage: 1,
    runsPageSize: 20,
    runsTotal: 0,
    // 两段式取消：待确认的 run id 存在 state 里，WS 触发的重绘不会丢状态
    pendingCancelId: null,
    failOnly: false,
    currentRun: null,
  };

  let ws = null;
  let searchDebounceTimer = null;
  let cancelConfirmTimer = null;

  // 创建/编辑向导状态（cp = create playbook）
  let cpState = { step: 1, totalSteps: 3, vars: [], tasks: [] };
  let cpTaskCounter = 0;
  let dragTaskIdx = -1;

  // 运行弹窗选择状态
  let runSel = { nodes: new Set(), groups: new Set(), tags: new Set() };
  let runVars = [];
  let runNodes = [];
  let runGroupCounts = {};

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  // 分类名哈希到 tag-rN 彩虹色（与 nodes 页分组同款算法，颜色稳定）
  function tagColor(s) {
    let h = 0;
    for (let i = 0; i < s.length; i++) h = ((h << 5) - h) + s.charCodeAt(i);
    return 'tag-r' + (Math.abs(h) % 12);
  }

  function showToast(msg, type) {
    const t = document.createElement('div');
    t.className = 'toast' + (type ? ' ' + type : '');
    t.textContent = msg;
    document.body.appendChild(t);
    setTimeout(() => { t.classList.add('show'); }, 10);
    setTimeout(() => { t.classList.remove('show'); setTimeout(() => t.remove(), 300); }, 2500);
  }

  function emptyView(icon, title, desc) {
    return `<div class="view-empty">
      <div class="empty-icon"><svg width="36" height="36" aria-hidden="true"><use href="#icon-${icon}"/></svg></div>
      <div class="empty-title">${esc(title)}</div>
      <div class="empty-desc">${esc(desc)}</div>
    </div>`;
  }

  function fmtTime(ts) { return ts ? new Date(ts).toLocaleString('zh-CN', { hour12: false }) : ''; }

  // 轻量 YAML 高亮：整行注释 / key: value / bool / 数字 / 引号字符串。
  // token 类复用 .yaml-preview（yaml-key/str/bool/num/comment）。
  function highlightYAML(src) {
    return String(src).split('\n').map(line => {
      let m = line.match(/^(\s*)(#.*)$/);
      if (m) return `${esc(m[1])}<span class="yaml-comment">${esc(m[2])}</span>`;
      m = line.match(/^(\s*(?:-\s+)?)([A-Za-z0-9_.\-\/]+):(?:\s+(.*))?$/);
      if (m) {
        const [, indent, key, val] = m;
        let v = '';
        if (val !== undefined) {
          const tv = val.trim();
          if (tv.startsWith('#')) v = ` <span class="yaml-comment">${esc(val)}</span>`;
          else if (/^["'].*["']$/.test(tv)) v = ` <span class="yaml-str">${esc(val)}</span>`;
          else if (tv === 'true' || tv === 'false') v = ` <span class="yaml-bool">${esc(val)}</span>`;
          else if (tv !== '' && !isNaN(Number(tv))) v = ` <span class="yaml-num">${esc(val)}</span>`;
          else v = ` ${esc(val)}`;
        }
        return `${esc(indent)}<span class="yaml-key">${esc(key)}</span>:${v}`;
      }
      return esc(line);
    }).join('\n');
  }

  // ==================== 数据加载 ====================

  async function loadSettingsPath() {
    try {
      const res = await api.playbookSettingsPath();
      const input = document.getElementById('playbook-path');
      if (res.value && input) input.value = res.value;
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
    // 刷新后选中项可能已被删除
    if (state.selectedId && !state.playbooks.some(p => p.id === state.selectedId)) {
      state.selectedId = null;
      renderDetailEmpty();
    }
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
    renderList();
    renderPanel();
  }

  // ==================== 左侧分类面板 ====================

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
        <li class="panel-item ${tagColor(c)} ${state.selectedCategory === c ? 'active' : ''}" data-category="${esc(c)}" role="option" aria-selected="${state.selectedCategory === c}">
          <span class="dot"></span>
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

  // ==================== 主栏：剧本列表 ====================

  function renderList() {
    const body = document.getElementById('pb-list-body');
    if (!body) return;
    const countEl = document.getElementById('pb-count');
    if (countEl) countEl.textContent = `${state.filteredPlaybooks.length}/${state.playbooks.length}`;

    if (state.playbooks.length === 0) {
      body.innerHTML = emptyView('scroll', '暂无剧本', '在「剧本库」中设置路径并同步，或点击「新建」创建第一个剧本');
      return;
    }
    if (state.filteredPlaybooks.length === 0) {
      body.innerHTML = emptyView('search', '无匹配剧本', '换个关键字或分类试试');
      return;
    }
    body.innerHTML = state.view === 'grid' ? renderGrid() : renderTable();
  }

  function categoryTag(pb) {
    return pb.category
      ? `<span class="tag ${tagColor(pb.category)}">${esc(pb.category)}</span>`
      : '<span class="cell-muted">-</span>';
  }

  function renderTable() {
    const rows = state.filteredPlaybooks.map(pb => `
      <tr class="playbook-row ${state.selectedId === pb.id ? 'selected' : ''}" data-pb-id="${esc(pb.id)}">
        <td class="cell-ellipsis" title="${esc(pb.name)}">${esc(pb.name)}${pb.file_exists === false ? ' <span class="missing-badge">缺失</span>' : ''}</td>
        <td>${categoryTag(pb)}</td>
        <td class="cell-ellipsis cell-muted" title="${esc(pb.description || '')}">${esc(pb.description || '')}</td>
        <td class="cell-ellipsis cell-muted" title="${esc((pb.task_names || []).join('、'))}">${esc((pb.task_names || []).join('、'))}</td>
        <td class="action-cell">
          <button class="btn btn-ghost btn-sm run-playbook-btn" data-id="${esc(pb.id)}" ${pb.file_exists === false ? 'disabled' : ''}>运行</button>
        </td>
      </tr>`).join('');
    return `<table class="data-table playbook-table">
      <thead><tr><th>名称</th><th>分类</th><th>描述</th><th>任务</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table>`;
  }

  function renderGrid() {
    const cards = state.filteredPlaybooks.map(pb => `
      <div class="playbook-card ${state.selectedId === pb.id ? 'selected' : ''}" data-pb-id="${esc(pb.id)}">
        <div class="pb-header">
          <span class="pb-name" title="${esc(pb.name)}">${esc(pb.name)}${pb.file_exists === false ? ' <span class="missing-badge">缺失</span>' : ''}</span>
          ${categoryTag(pb)}
        </div>
        <div class="pb-desc" title="${esc(pb.description || '')}">${esc(pb.description || '无描述')}</div>
        <div class="pb-meta">
          <span>${(pb.task_names || []).length} 任务</span>
          <span>v${esc(pb.version || '1.0')}</span>
          <button class="btn btn-ghost btn-sm run-playbook-btn" data-id="${esc(pb.id)}" ${pb.file_exists === false ? 'disabled' : ''} style="margin-left:auto">运行</button>
        </div>
      </div>`).join('');
    return `<div class="playbook-grid">${cards}</div>`;
  }

  function bindListEvents() {
    const body = document.getElementById('pb-list-body');
    if (!body || body.dataset.pbDelegated) return;
    body.dataset.pbDelegated = '1';
    body.addEventListener('click', (e) => {
      const runBtn = e.target.closest('.run-playbook-btn');
      if (runBtn) {
        e.stopPropagation();
        if (!runBtn.disabled) showRunModal(runBtn.dataset.id);
        return;
      }
      const row = e.target.closest('[data-pb-id]');
      if (row) selectPlaybook(row.dataset.pbId);
    });
  }

  function setView(v) {
    state.view = v;
    localStorage.setItem(PB_VIEW_KEY, v);
    const tableBtn = document.getElementById('pb-view-table');
    const gridBtn = document.getElementById('pb-view-grid');
    if (tableBtn) tableBtn.classList.toggle('active', v === 'table');
    if (gridBtn) gridBtn.classList.toggle('active', v === 'grid');
    renderList();
  }

  function setLibExpanded(open) {
    state.libExpanded = open;
    localStorage.setItem(PB_LIB_KEY, open ? '1' : '0');
    const row = document.getElementById('pb-lib-row');
    const btn = document.getElementById('pb-lib-toggle');
    if (row) row.hidden = !open;
    if (btn) btn.setAttribute('aria-expanded', String(open));
  }

  // ==================== 右栏：剧本详情 ====================

  function renderDetailEmpty() {
    const card = document.getElementById('pb-detail-card');
    if (card) card.innerHTML = emptyView('scroll', '选择一个剧本', '点击左侧列表查看剧本详情、任务与 YAML 源文件');
  }

  function selectPlaybook(id) {
    state.selectedId = id;
    document.querySelectorAll('#pb-list-body [data-pb-id]').forEach(el => {
      el.classList.toggle('selected', el.dataset.pbId === id);
    });
    const card = document.getElementById('pb-detail-card');
    if (!card) return;
    card.innerHTML = '<div class="view-loading">加载剧本详情…</div>';
    api.playbookGet(id).then(pb => {
      if (state.selectedId === id) renderDetail(pb);
    }).catch(() => {
      if (state.selectedId === id) card.innerHTML = emptyView('alert-circle', '加载失败', '无法获取剧本详情，请稍后重试');
    });
  }

  function renderDetail(pb) {
    const card = document.getElementById('pb-detail-card');
    if (!card) return;
    const missing = pb.file_exists === false;
    const tasks = pb.task_names || [];

    card.innerHTML = `
      <div class="panel-head">
        <div style="flex:1;min-width:0">
          <h3 class="panel-title" title="${esc(pb.name)}">${esc(pb.name)}${missing ? ' <span class="missing-badge">文件缺失</span>' : ''}</h3>
          <div class="panel-desc">${esc(pb.description || '无描述')}</div>
        </div>
        <div style="display:flex;gap:6px;flex-shrink:0">
          <button class="btn btn-primary btn-sm" id="detail-run-btn" ${missing ? 'disabled' : ''}><svg width="12" height="12" aria-hidden="true"><use href="#icon-play"/></svg> 运行</button>
          <button class="btn btn-secondary btn-sm" id="detail-edit-btn" ${missing ? 'disabled' : ''} title="二次编辑剧本">编辑</button>
          <button class="btn btn-ghost btn-sm" id="detail-download-btn" ${missing ? 'disabled' : ''} title="下载 playbook 文件">下载</button>
        </div>
      </div>
      <div class="panel-body" style="padding:14px 20px">
        <div class="pb-meta-grid">
          <div class="detail-field"><label>分类</label><div class="value">${categoryTag(pb)}</div></div>
          <div class="detail-field"><label>版本</label><div class="value">${esc(pb.version || '-')}</div></div>
          <div class="detail-field"><label>任务数</label><div class="value">${tasks.length}</div></div>
          <div class="detail-field"><label>文件状态</label><div class="value">${missing ? '<span style="color:var(--danger)">缺失</span>' : '<span style="color:var(--success)">存在</span>'}</div></div>
          <div class="detail-field pb-meta-file"><label>文件路径</label><div class="value cell-mono" style="white-space:normal;word-break:break-all">${esc(pb.file_path || '-')}</div></div>
        </div>
        ${tasks.length ? `
        <div style="margin:12px 0 4px">
          <div class="field-label" style="margin-bottom:6px">任务列表</div>
          <div style="display:flex;flex-wrap:wrap;gap:6px">${tasks.map(t => `<span class="tag ${tagColor(t)}">${esc(t)}</span>`).join('')}</div>
        </div>` : ''}
        <div style="margin-top:12px">
          <div class="field-label" style="margin-bottom:6px">YAML 源文件</div>
          <div id="detail-yaml" class="yaml-preview">加载中…</div>
        </div>
      </div>`;

    const runBtn = document.getElementById('detail-run-btn');
    if (runBtn) runBtn.addEventListener('click', () => showRunModal(pb.id));
    const editBtn = document.getElementById('detail-edit-btn');
    if (editBtn) editBtn.addEventListener('click', () => showEditModal(pb.id));
    const dlBtn = document.getElementById('detail-download-btn');
    if (dlBtn) dlBtn.addEventListener('click', async () => {
      try { await api.playbookDownload(pb.id); }
      catch (err) { showToast('下载失败: ' + err.message, 'error'); }
    });

    const yamlEl = document.getElementById('detail-yaml');
    if (missing) {
      yamlEl.textContent = '文件不存在';
    } else {
      api.playbookFile(pb.id).then(res => {
        yamlEl.innerHTML = highlightYAML(res.content || '(空文件)');
      }).catch(() => {
        yamlEl.textContent = `文件路径: ${pb.file_path}\n\n(YAML 加载失败)`;
      });
    }
  }

  // ==================== 右栏：运行历史 ====================

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
        // 两段式确认：第一击进入「确认取消」态（3 秒内有效），第二击才真正取消。
        // 状态存 state，WS 触发的整表重绘不会打断确认流程。
        if (state.pendingCancelId !== pending.id) {
          state.pendingCancelId = pending.id;
          clearTimeout(cancelConfirmTimer);
          cancelConfirmTimer = setTimeout(() => {
            state.pendingCancelId = null;
            renderRuns(state.runs);
          }, 3000);
          renderRuns(state.runs);
          return;
        }
        state.pendingCancelId = null;
        clearTimeout(cancelConfirmTimer);
        api.cancelPlaybookRun(pending.id)
          .then(loadRuns)
          .catch(err => showToast('取消失败: ' + err.message, 'error'));
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
    state.runs = runs || [];
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
        const cancellable = r.status === 'running' || r.status === 'pending' || r.status === 'queued';
        const confirming = state.pendingCancelId === r.id;
        return `<tr>
        <td class="cell-ellipsis" title="${esc(r.playbook_name)}">${esc(r.playbook_name)}</td>
        <td class="cell-ellipsis cell-muted" title="${esc(nodes.join(', '))}">${esc(nodesText)}</td>
        <td><span class="status-badge status-${esc(r.status)}">${esc(r.status)}</span></td>
        <td class="cell-muted">${fmtTime(r.created_at)}</td>
        <td class="action-cell">
          <button class="btn btn-ghost btn-sm view-run-btn" data-id="${r.id}">查看</button>
          ${cancellable ? `<button class="btn btn-ghost btn-sm cancel-run-btn" data-id="${r.id}" style="color:var(--danger)">${confirming ? '确认取消' : '取消'}</button>` : ''}
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
    state.currentRun = run;
    const detail = document.getElementById('run-detail');
    if (!detail) return;
    const nodes = run.target_nodes || [];
    const results = run.results || [];
    const total = results.length;
    const done = results.filter(r => r.status === 'completed' || r.status === 'success').length;
    const failed = results.filter(r => r.status === 'failed' || r.status === 'error').length;
    const shown = state.failOnly ? results.filter(r => r.status === 'failed' || r.status === 'error') : results;
    const pct = total ? Math.round(((done + failed) / total) * 100) : 0;

    const steps = shown.map(r => `<tr>
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
        <span class="run-meta-item"><span class="rm-label">目标节点</span><strong class="cell-ellipsis" style="max-width:480px" title="${esc(nodes.join(', '))}">${esc(nodes.join(', ') || '—')}</strong></span>
        <span class="run-meta-item"><span class="rm-label">开始时间</span><span class="cell-muted">${fmtTime(run.created_at)}</span></span>
        ${run.error ? `<span class="run-meta-item" style="color:var(--danger)">错误：${esc(run.error)}</span>` : ''}
      </div>
      <div class="run-progress-row">
        <div class="run-progress"><i style="width:${pct}%"></i></div>
        <span>${done + failed}/${total} 步 · 成功 ${done} · 失败 ${failed}</span>
        <label class="run-failonly"><input type="checkbox" id="run-fail-only" ${state.failOnly ? 'checked' : ''}> 仅看失败</label>
      </div>
      <div style="max-height:320px;overflow:auto;border:1px solid var(--border);border-radius:var(--radius)">
        <table class="run-steps">
          <colgroup><col style="width:20%"><col style="width:14%"><col style="width:12%"><col style="width:10%"><col style="width:7%"><col style="width:37%"></colgroup>
          <thead><tr><th>任务</th><th>节点</th><th>动作</th><th>状态</th><th>退出码</th><th>输出</th></tr></thead>
          <tbody>${steps || `<tr><td colspan="6" class="empty-state">${state.failOnly ? '没有失败步骤' : '暂无步骤结果'}</td></tr>`}</tbody>
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

  // ==================== 运行弹窗 ====================

  function showRunModal(id) {
    const pb = state.playbooks.find(p => p.id === id);
    runSel = { nodes: new Set(), groups: new Set(), tags: new Set() };
    runVars = [];
    document.getElementById('run-playbook-id').value = id;
    document.getElementById('run-playbook-name-display').textContent = pb ? pb.name : id;
    document.getElementById('run-playbook-target').value = '';
    document.getElementById('run-playbook-error').textContent = '';
    const warnEl = document.getElementById('run-playbook-warnings');
    if (warnEl) { warnEl.style.display = 'none'; warnEl.textContent = ''; }
    renderRunVars();
    updateRunNodeCount();
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
      renderRunNodeChips();
    } catch { renderRunGroups([]); renderRunNodeChips(); }

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

  function renderRunNodeChips() {
    const box = document.getElementById('run-node-chips');
    if (!box) return;
    if (!runNodes.length) {
      box.innerHTML = '<span class="field-hint">无可用节点</span>';
      return;
    }
    const matched = filterRunNodes(document.getElementById('run-playbook-target').value);
    if (!matched.length) {
      box.innerHTML = '<span class="field-hint">无匹配节点</span>';
      return;
    }
    box.innerHTML = matched.map(n => {
      const selected = runSel.nodes.has(n.id);
      const st = n.status === 'online' ? 'st-online' : n.status === 'offline' ? 'st-offline' : 'st-warn';
      const namePart = n.name && n.name !== n.id ? ` <span style="color:var(--muted)">(${esc(n.name)})</span>` : '';
      return `<button type="button" class="node-chip ${selected ? 'selected' : ''}" data-id="${esc(n.id)}" aria-pressed="${selected}" title="${esc(n.address || '')}">
        <span class="dot ${st}"></span>${esc(n.id)}${namePart}
      </button>`;
    }).join('');
  }

  function updateRunNodeCount() {
    const el = document.getElementById('run-node-count');
    if (el) el.textContent = String(runSel.nodes.size);
  }

  function renderRunGroups(groups) {
    const grid = document.getElementById('run-group-grid');
    if (!grid) return;
    if (!groups.length) {
      grid.innerHTML = '<span class="field-hint">暂无分组</span>';
      return;
    }
    grid.innerHTML = groups.map(g => {
      const selected = runSel.groups.has(g);
      return `<button type="button" class="group-chip ${tagColor(g)} ${selected ? 'selected' : ''}" data-group="${esc(g)}" aria-pressed="${selected}">
        <span class="dot"></span><span class="group-text">${esc(g)}</span><span class="count">${runGroupCounts[g] || 0}</span>
      </button>`;
    }).join('');
  }

  function renderRunTags(tags) {
    const grid = document.getElementById('run-tag-grid');
    if (!grid) return;
    if (!tags.length) {
      grid.innerHTML = '<span class="field-hint">该剧本无标签任务</span>';
      return;
    }
    grid.innerHTML = tags.map(t => {
      const selected = runSel.tags.has(t);
      return `<button type="button" class="group-chip ${tagColor(t)} ${selected ? 'selected' : ''}" data-tag="${esc(t)}" aria-pressed="${selected}">
        <span class="dot"></span><span class="group-text">${esc(t)}</span>
      </button>`;
    }).join('');
  }

  function renderRunVars() {
    const list = document.getElementById('run-vars-list');
    if (!list) return;
    if (!runVars.length) {
      list.innerHTML = '<span class="field-hint">未设置额外变量（可选）</span>';
      return;
    }
    list.innerHTML = '';
    runVars.forEach((v, idx) => {
      const row = document.createElement('div');
      row.className = 'kv-row';
      const keyInp = document.createElement('input');
      keyInp.className = 'run-var-key';
      keyInp.placeholder = '变量名';
      keyInp.style.flex = '1';
      keyInp.value = v.key;
      keyInp.addEventListener('input', () => { runVars[idx].key = keyInp.value; });
      const valInp = document.createElement('input');
      valInp.className = 'run-var-value';
      valInp.placeholder = '值';
      valInp.style.flex = '2';
      valInp.value = v.value;
      valInp.addEventListener('input', () => { runVars[idx].value = valInp.value; });
      const delBtn = document.createElement('button');
      delBtn.type = 'button';
      delBtn.textContent = '删除';
      delBtn.className = 'btn-outline-danger';
      delBtn.addEventListener('click', () => { runVars.splice(idx, 1); renderRunVars(); });
      row.append(keyInp, valInp, delBtn);
      list.appendChild(row);
    });
  }

  async function loadRunStagingFiles() {
    const listEl = document.getElementById('run-staging-files');
    const dirEl = document.getElementById('run-staging-dir');
    if (!listEl) return;
    listEl.innerHTML = '加载中…';
    try {
      const [fRes, dRes] = await Promise.all([api.staging.files(), api.staging.disk()]);
      const files = fRes.data || [];
      if (dRes && dRes.staging_dir) dirEl.textContent = dRes.staging_dir;
      if (files.length === 0) {
        listEl.innerHTML = '<span class="field-hint">暂无文件，可先在「文件」页上传</span>';
        return;
      }
      listEl.innerHTML = files.map(f => `
        <div class="stg-row">
          <span class="stg-name">${esc(f.name)} <span style="color:var(--muted)">(${f.size} B)</span></span>
          <button type="button" class="staging-copy-path btn btn-ghost btn-sm" data-name="${esc(f.name)}">复制路径</button>
        </div>`).join('');
    } catch {
      listEl.innerHTML = '<span style="color:var(--danger);font-size:var(--fs-xs)">中转站文件加载失败</span>';
    }
  }

  // ==================== 创建/编辑向导 ====================

  // 每种 action 的专属字段；不在模板内的参数以自定义键值对呈现
  const ACTION_FIELDS = {
    command: [
      { k: 'cmd', label: '命令', type: 'textarea', ph: 'uptime' },
    ],
    script: [
      { k: 'script', label: '脚本路径（中转站）', ph: 'deploy.sh' },
      { k: 'dest', label: '远程目录', ph: '/tmp/' },
      { k: 'args', label: '脚本参数', ph: '可选' },
    ],
    upload: [
      { k: 'src', label: '本地路径（中转站）', ph: 'app.tar.gz' },
      { k: 'dest', label: '远程路径', ph: '/opt/app/' },
      { k: 'overwrite', label: '覆盖已存在文件', type: 'bool' },
    ],
    download: [
      { k: 'src', label: '远程路径', ph: '/var/log/app.log' },
      { k: 'dest', label: '本地目录', ph: 'downloads/' },
      { k: 'subdir', label: '按节点建子目录', type: 'bool' },
    ],
    include: [
      { k: 'playbook', label: '剧本路径', ph: 'other.yaml' },
    ],
  };

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

  function resetCpModal() {
    cpState = { step: 1, totalSteps: 3, vars: [], tasks: [] };
    cpTaskCounter = 0;
    dragTaskIdx = -1;
    document.getElementById('cp-name').value = '';
    document.getElementById('cp-category').value = '';
    document.getElementById('cp-desc').value = '';
    document.getElementById('cp-version').value = '1.0';
    document.getElementById('cp-mode').value = '';
    document.getElementById('cp-groups').value = '';
    document.getElementById('cp-tags').value = '';
    document.getElementById('cp-skip-tags').value = '';
    document.getElementById('cp-vars-list').innerHTML = '';
    document.getElementById('cp-tasks-list').innerHTML = '<p class="empty-state" style="font-size:var(--fs-sm);padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>';
    document.getElementById('cp-error').textContent = '';
    showCpStep(1);
  }

  async function showEditModal(id) {
    let data;
    try {
      data = await api.playbookEdit(id);
    } catch (e) { showToast('加载剧本失败: ' + e.message, 'error'); console.error(e); return; }

    try {
      resetCpModal();
      document.getElementById('cp-title').textContent = '✏️ 编辑剧本: ' + (data.name || id);
      document.getElementById('cp-name').value = data.name || '';
      document.getElementById('cp-category').value = data.category || '';
      document.getElementById('cp-desc').value = data.description || '';
      document.getElementById('cp-version').value = data.version || '1.0';
      document.getElementById('cp-mode').value = data.execution_mode || '';
      document.getElementById('cp-groups').value = (data.default_groups || []).join(',');
      document.getElementById('cp-tags').value = (data.default_tags || []).join(',');
      document.getElementById('cp-skip-tags').value = (data.default_skip_tags || []).join(',');

      cpState.vars = Object.entries(data.vars || {}).map(([k, v]) => ({ key: k, value: String(v) }));
      cpState.tasks = (data.tasks || []).map(t => ({ name: t.name, action: t.action, args: t.args || {} }));
      cpState.preTasks = data.pre_tasks || [];
      cpState.postTasks = data.post_tasks || [];
      cpTaskCounter = cpState.tasks.length;
      renderCpVars();
      renderCpTasks();

      document.getElementById('create-playbook-modal').classList.add('open');
    } catch (e) {
      showToast('打开编辑窗口失败: ' + e.message, 'error');
      console.error(e);
    }
  }

  function showCpStep(n) {
    cpState.step = n;
    document.querySelectorAll('.create-pb-page').forEach(el => el.style.display = 'none');
    document.querySelector(`.create-pb-page[data-page="${n}"]`).style.display = 'block';
    document.querySelectorAll('.wiz-step').forEach(el => {
      const s = parseInt(el.dataset.step);
      el.classList.toggle('active', s === n);
      el.classList.toggle('done', s < n);
    });
    document.querySelectorAll('.step-dot').forEach(el => el.classList.toggle('active', parseInt(el.dataset.step) <= n));
    document.getElementById('cp-prev-btn').style.display = n > 1 ? '' : 'none';
    document.getElementById('cp-next-btn').style.display = n < cpState.totalSteps ? '' : 'none';
    document.getElementById('cp-save-btn').style.display = n === cpState.totalSteps ? '' : 'none';
    if (n === cpState.totalSteps) buildCpSummary();
    document.getElementById('cp-error').textContent = '';
  }

  function renderCpVars() {
    const list = document.getElementById('cp-vars-list');
    list.innerHTML = '';
    if (!cpState.vars.length) {
      list.innerHTML = '<span class="field-hint">剧本无需变量时可直接跳过</span>';
      return;
    }
    cpState.vars.forEach((v, idx) => {
      const row = document.createElement('div');
      row.className = 'kv-row';
      row.style.marginBottom = '6px';
      const keyInp = document.createElement('input');
      keyInp.className = 'cp-var-key';
      keyInp.placeholder = '变量名';
      keyInp.style.flex = '1';
      keyInp.value = v.key;
      keyInp.addEventListener('input', () => { cpState.vars[idx].key = keyInp.value; });
      const valInp = document.createElement('input');
      valInp.className = 'cp-var-value';
      valInp.placeholder = '值';
      valInp.style.flex = '2';
      valInp.value = v.value;
      valInp.addEventListener('input', () => { cpState.vars[idx].value = valInp.value; });
      const delBtn = document.createElement('button');
      delBtn.type = 'button';
      delBtn.textContent = '删除';
      delBtn.className = 'btn-outline-danger';
      delBtn.addEventListener('click', () => { cpState.vars.splice(idx, 1); renderCpVars(); });
      row.append(keyInp, valInp, delBtn);
      list.appendChild(row);
    });
  }

  function actionKeys(action) {
    return new Set((ACTION_FIELDS[action] || []).map(f => f.k));
  }

  function renderCpTasks() {
    const list = document.getElementById('cp-tasks-list');
    if (!list) return;
    if (cpState.tasks.length === 0) {
      list.innerHTML = '<p class="empty-state" style="font-size:var(--fs-sm);padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>';
      return;
    }
    const actions = Object.keys(ACTION_FIELDS);
    list.innerHTML = cpState.tasks.map((t, i) => {
      const fields = ACTION_FIELDS[t.action] || [];
      const typedKeys = actionKeys(t.action);
      const customEntries = Object.entries(t.args || {}).filter(([k]) => !typedKeys.has(k));
      const fieldHtml = fields.map(f => {
        const val = (t.args || {})[f.k];
        if (f.type === 'bool') {
          return `<label class="cp-field cp-field-bool">
            <input type="checkbox" data-tf-bool="${i}:${esc(f.k)}" ${val === true ? 'checked' : ''} style="width:auto">
            <span>${esc(f.label)}</span>
          </label>`;
        }
        if (f.type === 'textarea') {
          return `<div class="cp-field">
            <label>${esc(f.label)}</label>
            <textarea rows="2" data-tf="${i}:${esc(f.k)}" placeholder="${esc(f.ph || '')}">${esc(val !== undefined && val !== null ? String(val) : '')}</textarea>
          </div>`;
        }
        return `<div class="cp-field">
          <label>${esc(f.label)}</label>
          <input data-tf="${i}:${esc(f.k)}" placeholder="${esc(f.ph || '')}" value="${esc(val !== undefined && val !== null ? String(val) : '')}" spellcheck="false">
        </div>`;
      }).join('');
      const customHtml = customEntries.map(([k, v]) => `
        <div class="kv-row">
          <input class="cp-arg-key" data-arg-key="${i}:${esc(k)}" placeholder="参数名" value="${esc(k)}" style="width:130px;flex:none">
          <input class="cp-arg-value" data-arg-value="${i}:${esc(k)}" placeholder="值" value="${esc(typeof v === 'string' ? v : JSON.stringify(v))}" style="flex:1">
          <button type="button" class="btn-outline-danger cp-arg-del" data-arg-del="${i}:${esc(k)}">×</button>
        </div>`).join('');
      const actionOptions = actions.map(a => `<option value="${a}" ${t.action === a ? 'selected' : ''}>${a}</option>`).join('')
        + (actions.includes(t.action) ? '' : `<option value="${esc(t.action)}" selected>${esc(t.action)}</option>`);
      return `<div class="cp-task-card" draggable="true" data-idx="${i}">
        <div class="cp-task-head">
          <span class="drag-handle" title="拖拽排序">⠿</span>
          <input class="cp-task-name" data-name="${i}" placeholder="任务名称" value="${esc(t.name || '')}" style="flex:1">
          <select data-action-idx="${i}" style="width:120px;flex:none">${actionOptions}</select>
          <button type="button" class="btn-outline-danger" data-del-idx="${i}" title="删除任务">删除</button>
        </div>
        <div class="cp-task-body">
          ${fieldHtml}
          ${customHtml}
          <div class="kv-row" style="justify-content:space-between">
            <button type="button" class="btn btn-ghost btn-sm" data-add-arg="${i}">+ 自定义参数</button>
            <span style="display:flex;gap:4px">
              <button type="button" class="btn btn-ghost btn-sm" data-move="${i}:-1" ${i === 0 ? 'disabled' : ''} title="上移">↑</button>
              <button type="button" class="btn btn-ghost btn-sm" data-move="${i}:1" ${i === cpState.tasks.length - 1 ? 'disabled' : ''} title="下移">↓</button>
            </span>
          </div>
        </div>
      </div>`;
    }).join('');
  }

  function bindCpTaskEvents() {
    const list = document.getElementById('cp-tasks-list');
    if (!list || list.dataset.cpDelegated) return;
    list.dataset.cpDelegated = '1';

    const parseRef = (s) => {
      const sep = s.indexOf(':');
      return [parseInt(s.slice(0, sep)), s.slice(sep + 1)];
    };

    list.addEventListener('input', (e) => {
      const tf = e.target.closest('[data-tf]');
      if (tf) { const [i, k] = parseRef(tf.dataset.tf); cpState.tasks[i].args[k] = tf.value; return; }
      const name = e.target.closest('[data-name]');
      if (name) { cpState.tasks[parseInt(name.dataset.name)].name = name.value; return; }
      const argVal = e.target.closest('[data-arg-value]');
      if (argVal) { const [i, k] = parseRef(argVal.dataset.argValue); cpState.tasks[i].args[k] = argVal.value; }
    });

    list.addEventListener('change', (e) => {
      const boolField = e.target.closest('[data-tf-bool]');
      if (boolField) { const [i, k] = parseRef(boolField.dataset.tfBool); cpState.tasks[i].args[k] = boolField.checked; return; }
      const actionSel = e.target.closest('[data-action-idx]');
      if (actionSel) { onActionChange(parseInt(actionSel.dataset.actionIdx), actionSel.value); return; }
      const argKey = e.target.closest('[data-arg-key]');
      if (argKey) {
        const [i, oldKey] = parseRef(argKey.dataset.argKey);
        renameArg(cpState.tasks[i].args, oldKey, argKey.value);
        renderCpTasks();
      }
    });

    list.addEventListener('click', (e) => {
      const del = e.target.closest('[data-del-idx]');
      if (del) { cpState.tasks.splice(parseInt(del.dataset.delIdx), 1); renderCpTasks(); return; }
      const addArg = e.target.closest('[data-add-arg]');
      if (addArg) {
        const i = parseInt(addArg.dataset.addArg);
        cpState.tasks[i].args[''] = '';
        renderCpTasks();
        return;
      }
      const argDel = e.target.closest('[data-arg-del]');
      if (argDel) {
        const [i, k] = parseRef(argDel.dataset.argDel);
        delete cpState.tasks[i].args[k];
        renderCpTasks();
        return;
      }
      const move = e.target.closest('[data-move]');
      if (move) {
        const [i, dir] = move.dataset.move.split(':').map(Number);
        const j = i + dir;
        if (j >= 0 && j < cpState.tasks.length) {
          [cpState.tasks[i], cpState.tasks[j]] = [cpState.tasks[j], cpState.tasks[i]];
          renderCpTasks();
        }
      }
    });

    // 拖拽排序：dragstart/dragover/drop 均冒泡，可委托到列表容器
    list.addEventListener('dragstart', (e) => {
      const card = e.target.closest('.cp-task-card');
      if (!card) return;
      dragTaskIdx = parseInt(card.dataset.idx);
      card.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      try { e.dataTransfer.setData('text/plain', String(dragTaskIdx)); } catch {}
    });
    list.addEventListener('dragover', (e) => {
      const card = e.target.closest('.cp-task-card');
      if (!card || dragTaskIdx < 0) return;
      e.preventDefault();
      card.classList.add('drag-over');
    });
    list.addEventListener('dragleave', (e) => {
      const card = e.target.closest('.cp-task-card');
      if (card) card.classList.remove('drag-over');
    });
    list.addEventListener('drop', (e) => {
      const card = e.target.closest('.cp-task-card');
      if (!card || dragTaskIdx < 0) return;
      e.preventDefault();
      const to = parseInt(card.dataset.idx);
      if (to !== dragTaskIdx) {
        const [moved] = cpState.tasks.splice(dragTaskIdx, 1);
        cpState.tasks.splice(to, 0, moved);
      }
      dragTaskIdx = -1;
      renderCpTasks();
    });
    list.addEventListener('dragend', () => {
      dragTaskIdx = -1;
      list.querySelectorAll('.cp-task-card').forEach(c => c.classList.remove('dragging', 'drag-over'));
    });
  }

  // 切换任务类型：保留两边模板都有的参数值，其余回到新类型的默认值
  function onActionChange(i, newAction) {
    const t = cpState.tasks[i];
    const oldArgs = t.args || {};
    const defaults = getActionArgs(newAction);
    const merged = {};
    for (const k of Object.keys(defaults)) {
      merged[k] = (k in oldArgs) ? oldArgs[k] : defaults[k];
    }
    t.action = newAction;
    t.args = merged;
    renderCpTasks();
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
    const groups = document.getElementById('cp-groups').value.trim();
    const tags = document.getElementById('cp-tags').value.trim();
    const skipTags = document.getElementById('cp-skip-tags').value.trim();
    const vars = cpState.vars.filter(v => v.key.trim());

    document.getElementById('cp-summary').innerHTML = `
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:6px 16px;font-size:var(--fs-sm)">
        <div><strong>名称:</strong> ${esc(name)}</div>
        <div><strong>版本:</strong> ${esc(version)}</div>
        <div><strong>描述:</strong> ${esc(desc || '-')}</div>
        <div><strong>分类:</strong> ${esc(category || '-')}</div>
        <div><strong>执行模式:</strong> ${esc(mode)}</div>
        <div><strong>变量:</strong> ${vars.length} 项</div>
        <div><strong>任务:</strong> ${cpState.tasks.length} 项</div>
        <div><strong>默认分组:</strong> ${esc(groups || '-')}</div>
        <div><strong>执行标签:</strong> ${esc(tags || '-')}</div>
        <div><strong>跳过标签:</strong> ${esc(skipTags || '-')}</div>
      </div>`;

    // 预览 YAML（仅展示用——真实 YAML 由服务端生成）
    const lines = [`name: ${name}`];
    if (category) lines.push(`category: ${category}`);
    if (desc) lines.push(`description: ${desc}`);
    lines.push(`version: "${version}"`, 'hosts: []');
    if (mode) lines.push(`execution_mode: ${mode}`);
    if (groups) lines.push(`default_groups: [${groups}]`);
    if (tags) lines.push(`default_tags: [${tags}]`);
    if (skipTags) lines.push(`default_skip_tags: [${skipTags}]`);
    if (vars.length) {
      lines.push('vars:');
      for (const v of vars) lines.push(`  ${v.key.trim()}: ${v.value}`);
    }
    lines.push('pre_tasks: []', 'tasks:');
    for (const t of cpState.tasks) {
      lines.push(`  - name: ${t.name}`, `    action: ${t.action}`, '    args:');
      for (const [k, v] of Object.entries(t.args)) {
        const val = typeof v === 'string' ? v : JSON.stringify(v);
        lines.push(`      ${k}: ${val}`);
      }
    }
    lines.push('post_tasks: []');
    document.getElementById('cp-preview').innerHTML = highlightYAML(lines.join('\n'));
  }

  // ==================== 页面骨架 ====================

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
    <div class="pb-layout">
      <!-- 左栏：剧本列表 -->
      <div class="section-card pb-master">
        <div class="pb-toolbar">
          <div class="pb-toolbar-row">
            <h3 class="pb-title">剧本列表 <span class="count" id="pb-count">0</span></h3>
            <button class="btn btn-ghost btn-sm" id="pb-lib-toggle" aria-expanded="${state.libExpanded}" title="剧本库路径设置"><svg width="14" height="14" aria-hidden="true"><use href="#icon-settings"/></svg> 剧本库</button>
            <span style="flex:1"></span>
            <button class="btn btn-secondary btn-sm" id="upload-playbook-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-upload"/></svg> 上传</button>
            <button class="btn btn-primary btn-sm" id="add-playbook-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 新建</button>
          </div>
          <div class="pb-lib-row" id="pb-lib-row" ${state.libExpanded ? '' : 'hidden'}>
            <div class="path-bar">
              <label for="playbook-path">Library Path</label>
              <div class="path-group">
                <input id="playbook-path" placeholder="/path/to/playbooks" spellcheck="false">
                <button class="btn btn-secondary btn-sm" id="refresh-playbooks-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg> 同步</button>
              </div>
            </div>
          </div>
          <div class="pb-toolbar-row">
            <div class="pb-search">
              <svg width="14" height="14" aria-hidden="true"><use href="#icon-search"/></svg>
              <input type="text" id="playbook-search" placeholder="搜索剧本名称 / 描述…">
            </div>
            <div class="seg" role="group" aria-label="视图切换">
              <button type="button" id="pb-view-table" class="${state.view === 'table' ? 'active' : ''}">表格</button>
              <button type="button" id="pb-view-grid" class="${state.view === 'grid' ? 'active' : ''}">卡片</button>
            </div>
          </div>
        </div>
        <div class="pb-list-body" id="pb-list-body">
          <p class="empty-state">加载中…</p>
        </div>
      </div>

      <!-- 右栏：剧本详情 -->
      <div class="pb-detail-col">
        <div class="section-card" id="pb-detail-card">
          ${emptyView('scroll', '选择一个剧本', '点击左侧列表查看剧本详情、任务与 YAML 源文件')}
        </div>
      </div>
    </div>

    <!-- 运行历史/运行详情：pb-layout 分栏之外全宽渲染。右栏分到的半宽放不下
         运行详情的 meta 行 + 6 列步骤结果表（表头竖排、输出列成缝） -->
    <div class="section-card" id="pb-runs-card">
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

    <input type="file" id="upload-playbook-file" accept=".yaml,.yml" style="display:none">

    <!-- 运行弹窗 -->
    <div class="modal-overlay" id="run-playbook-modal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3 style="margin:0">运行剧本: <span id="run-playbook-name-display"></span></h3>
          <button type="button" class="btn btn-ghost btn-sm" id="run-playbook-close" style="margin-left:auto;background:none;border:none;color:var(--muted);cursor:pointer;font-size:var(--fs-xl)">&times;</button>
        </div>
        <div class="modal-form">
          <input type="hidden" id="run-playbook-id">
          <div class="form-row">
            <label>目标节点</label>
            <input id="run-playbook-target" placeholder="搜索节点 id / 名称 / 地址…" autocomplete="off">
            <div class="node-selector" id="run-node-chips" style="margin-top:6px"></div>
            <div class="node-select-info" style="margin-top:6px">
              <span>已选 <span class="count" id="run-node-count">0</span> 个节点</span>
              <span style="display:flex;gap:6px">
                <button type="button" id="run-node-all">全选</button>
                <button type="button" id="run-node-clear">清空</button>
              </span>
            </div>
          </div>
          <div class="form-row">
            <label>分组 <span class="lbl-hint">(与节点同时选择时以节点为准)</span></label>
            <div id="run-group-grid" class="node-selector"></div>
          </div>
          <div class="form-row">
            <label>标签 <span class="lbl-hint">(任务执行标签，可选)</span></label>
            <div id="run-tag-grid" class="node-selector"></div>
          </div>
          <div class="form-row">
            <label>Extra Vars <span class="lbl-hint">(key=value 追加到剧本变量)</span></label>
            <div id="run-vars-list"></div>
            <button type="button" class="btn btn-secondary btn-sm" id="run-add-var" style="margin-top:6px">+ 添加变量</button>
          </div>
          <div class="adv-toggle" id="run-staging-toggle">
            <span class="arrow" id="run-staging-arrow">▶</span> 中转站文件（upload/script 引用时可选用）
          </div>
          <div class="adv-options" id="run-staging-panel" style="flex-direction:column;align-items:stretch">
            <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:6px">
              <span class="field-hint">可先在「文件」页上传，再复制路径填入任务参数</span>
              <button type="button" class="btn btn-secondary btn-sm" id="run-staging-refresh" style="padding:1px 8px">刷新</button>
            </div>
            <div id="run-staging-files" style="max-height:160px;overflow-y:auto"></div>
            <div id="run-staging-dir" class="field-hint" style="margin-top:6px;word-break:break-all"></div>
          </div>
        </div>
        <p class="error-msg" id="run-playbook-error"></p>
        <p class="error-msg" id="run-playbook-warnings" style="display:none"></p>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="run-playbook-cancel">取消</button>
          <button class="btn btn-primary" id="run-playbook-submit">执行</button>
        </div>
      </div>
    </div>

    <!-- 创建/编辑向导 -->
    <div class="modal-overlay" id="create-playbook-modal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3 id="cp-title" style="margin:0">📝 创建剧本</h3>
          <button type="button" class="btn btn-ghost btn-sm" id="create-pb-close-btn" style="margin-left:auto;background:none;border:none;color:var(--muted);cursor:pointer;font-size:var(--fs-xl)">&times;</button>
        </div>
        <div class="modal-body" id="create-pb-body" style="max-height:70vh;overflow-y:auto">
          <div class="wiz-steps" id="create-pb-steps">
            <button type="button" class="wiz-step active" data-step="1"><span class="step-dot active" data-step="1">1</span>基本信息</button>
            <span class="wiz-arrow">›</span>
            <button type="button" class="wiz-step" data-step="2"><span class="step-dot" data-step="2">2</span>任务与变量</button>
            <span class="wiz-arrow">›</span>
            <button type="button" class="wiz-step" data-step="3"><span class="step-dot" data-step="3">3</span>确认</button>
          </div>

          <!-- Step 1: 基本信息 + 执行配置 -->
          <div class="create-pb-page" data-page="1">
            <div class="field-grid">
              <div class="field"><label class="field-label">剧本名称 *</label><input id="cp-name" placeholder="my-playbook"><p class="field-hint">作为文件名与调用标识</p></div>
              <div class="field"><label class="field-label">分类</label><input id="cp-category" placeholder="web, db, cache…（可选）"></div>
              <div class="field"><label class="field-label">版本</label><input id="cp-version" value="1.0"></div>
              <div class="field"><label class="field-label">执行模式</label>
                <select id="cp-mode">
                  <option value="">fail_continue（失败继续）</option>
                  <option value="pipeline">pipeline（失败终止）</option>
                </select>
              </div>
              <div class="field field-wide"><label class="field-label">描述</label><textarea id="cp-desc" rows="2" placeholder="可选"></textarea></div>
            </div>
            <h4 class="cp-subtitle">默认配置（可选）</h4>
            <div class="field-grid">
              <div class="field"><label class="field-label">目标分组</label><input id="cp-groups" placeholder="web, db（逗号分隔）"></div>
              <div class="field"><label class="field-label">执行标签</label><input id="cp-tags" placeholder="tag1, tag2"></div>
              <div class="field"><label class="field-label">跳过标签</label><input id="cp-skip-tags" placeholder="skip-me"></div>
            </div>
          </div>

          <!-- Step 2: 任务与变量 -->
          <div class="create-pb-page" data-page="2" style="display:none">
            <h4 class="cp-subtitle">变量</h4>
            <div id="cp-vars-list"></div>
            <button type="button" class="btn btn-secondary btn-sm" id="cp-add-var">+ 添加变量</button>
            <h4 class="cp-subtitle" style="margin-top:20px">任务</h4>
            <div style="display:flex;gap:8px;align-items:flex-end">
              <div style="flex:1">
                <label class="field-label">任务类型</label>
                <select id="cp-task-action">
                  <option value="command">command — 执行 Shell 命令</option>
                  <option value="script">script — 执行脚本文件</option>
                  <option value="upload">upload — 上传文件到节点</option>
                  <option value="download">download — 从节点下载文件</option>
                  <option value="include">include — 包含其他剧本</option>
                </select>
              </div>
              <button type="button" class="btn btn-primary btn-sm" id="cp-add-task" style="white-space:nowrap">+ 添加</button>
            </div>
            <div id="cp-tasks-list" style="margin-top:12px"></div>
          </div>

          <!-- Step 3: 确认 -->
          <div class="create-pb-page" data-page="3" style="display:none">
            <div id="cp-summary" style="font-size:var(--fs-sm);margin-bottom:12px"></div>
            <h4 class="cp-subtitle">YAML 预览</h4>
            <div id="cp-preview" class="yaml-preview" style="max-height:300px;overflow:auto"></div>
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

    // ---- 列表工具栏 ----
    bindListEvents();
    document.getElementById('pb-lib-toggle').addEventListener('click', () => setLibExpanded(!state.libExpanded));
    setLibExpanded(state.libExpanded);

    document.getElementById('pb-view-table').addEventListener('click', () => setView('table'));
    document.getElementById('pb-view-grid').addEventListener('click', () => setView('grid'));

    document.getElementById('playbook-search').addEventListener('input', (e) => {
      clearTimeout(searchDebounceTimer);
      searchDebounceTimer = setTimeout(() => {
        state.query = e.target.value.trim();
        applyFilters();
      }, 100);
    });

    document.getElementById('refresh-playbooks-btn').addEventListener('click', async () => {
      const path = document.getElementById('playbook-path').value.trim();
      if (!path) { showToast('请先填写 Library Path', 'error'); return; }
      const btn = document.getElementById('refresh-playbooks-btn');
      btn.classList.add('loading');
      btn.disabled = true;
      try {
        const res = await api.refreshPlaybooks(path);
        if (res.errors && res.errors.length > 0) {
          showToast(`同步完成，${res.errors.length} 个文件解析失败`, 'error');
          console.error('playbook sync errors:', res.errors);
        } else {
          showToast('剧本库已同步', 'success');
        }
        loadAll();
      } catch (e) { showToast('同步失败: ' + e.message, 'error'); }
      btn.classList.remove('loading');
      btn.disabled = false;
    });

    // ---- 上传 ----
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
        showToast('上传成功', 'success');
        loadAll();
      } catch (err) { showToast('上传失败: ' + err.message, 'error'); }
      btn.innerHTML = '<svg width="14" height="14" aria-hidden="true"><use href="#icon-upload"/></svg> 上传';
      btn.disabled = false;
    });

    // ---- 运行历史分页 ----
    document.getElementById('runs-prev-btn').addEventListener('click', () => {
      if (state.runsPage > 1) { state.runsPage--; loadRuns(); }
    });
    document.getElementById('runs-next-btn').addEventListener('click', () => {
      if (state.runsPage < runsTotalPages()) { state.runsPage++; loadRuns(); }
    });

    // ---- 运行详情「仅看失败」筛选（run-detail 容器是静态元素，重绘不换壳）----
    document.getElementById('run-detail').addEventListener('change', (e) => {
      if (e.target && e.target.id === 'run-fail-only') {
        state.failOnly = e.target.checked;
        if (state.currentRun) showRunDetail(state.currentRun);
      }
    });

    // ---- 运行弹窗 ----
    const closeRunModal = () => document.getElementById('run-playbook-modal').classList.remove('open');
    document.getElementById('run-playbook-close').addEventListener('click', closeRunModal);
    document.getElementById('run-playbook-cancel').addEventListener('click', closeRunModal);
    document.getElementById('run-playbook-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) closeRunModal();
    });

    document.getElementById('run-playbook-target').addEventListener('input', renderRunNodeChips);
    document.getElementById('run-node-chips').addEventListener('click', (e) => {
      const chip = e.target.closest('.node-chip');
      if (!chip) return;
      if (runSel.nodes.has(chip.dataset.id)) runSel.nodes.delete(chip.dataset.id);
      else runSel.nodes.add(chip.dataset.id);
      renderRunNodeChips();
      updateRunNodeCount();
    });
    document.getElementById('run-node-all').addEventListener('click', () => {
      for (const n of filterRunNodes(document.getElementById('run-playbook-target').value)) {
        runSel.nodes.add(n.id);
      }
      renderRunNodeChips();
      updateRunNodeCount();
    });
    document.getElementById('run-node-clear').addEventListener('click', () => {
      runSel.nodes.clear();
      renderRunNodeChips();
      updateRunNodeCount();
    });
    document.getElementById('run-group-grid').addEventListener('click', (e) => {
      const chip = e.target.closest('.group-chip');
      if (!chip) return;
      const g = chip.dataset.group;
      if (runSel.groups.has(g)) runSel.groups.delete(g);
      else runSel.groups.add(g);
      chip.classList.toggle('selected');
      chip.setAttribute('aria-pressed', String(runSel.groups.has(g)));
    });
    document.getElementById('run-tag-grid').addEventListener('click', (e) => {
      const chip = e.target.closest('.group-chip');
      if (!chip) return;
      const t = chip.dataset.tag;
      if (runSel.tags.has(t)) runSel.tags.delete(t);
      else runSel.tags.add(t);
      chip.classList.toggle('selected');
      chip.setAttribute('aria-pressed', String(runSel.tags.has(t)));
    });

    document.getElementById('run-add-var').addEventListener('click', () => {
      runVars.push({ key: '', value: '' });
      renderRunVars();
    });

    document.getElementById('run-staging-toggle').addEventListener('click', () => {
      const panel = document.getElementById('run-staging-panel');
      const arrow = document.getElementById('run-staging-arrow');
      panel.classList.toggle('open');
      arrow.classList.toggle('open');
    });
    document.getElementById('run-staging-files').addEventListener('click', async (e) => {
      const btn = e.target.closest('.staging-copy-path');
      if (!btn) return;
      const dir = (document.getElementById('run-staging-dir').textContent || '').trim();
      const full = dir ? `${dir.replace(/[\\/]$/, '')}/${btn.dataset.name}` : btn.dataset.name;
      try {
        await navigator.clipboard.writeText(full);
        btn.textContent = '已复制';
        setTimeout(() => { btn.textContent = '复制路径'; }, 1500);
      } catch {
        showToast('复制失败，路径: ' + full, 'error');
      }
    });
    document.getElementById('run-staging-refresh').addEventListener('click', loadRunStagingFiles);

    document.getElementById('run-playbook-submit').addEventListener('click', async () => {
      const id = document.getElementById('run-playbook-id').value;
      if (runSel.nodes.size === 0 && runSel.groups.size === 0) {
        document.getElementById('run-playbook-error').textContent = '请选择至少一个目标节点或分组';
        return;
      }
      const body = {};
      if (runSel.nodes.size > 0) body.target_nodes = Array.from(runSel.nodes);
      if (runSel.groups.size > 0) body.groups = Array.from(runSel.groups);
      if (runSel.tags.size > 0) body.tags = Array.from(runSel.tags).join(',');
      const extraVars = {};
      for (const v of runVars) {
        if (v.key.trim()) extraVars[v.key.trim()] = v.value.trim();
      }
      if (Object.keys(extraVars).length) body.extra_vars = extraVars;
      try {
        const res = await api.runPlaybook(id, body);
        const warnings = (res && res.warnings) || [];
        if (warnings.length > 0) {
          const warnEl = document.getElementById('run-playbook-warnings');
          warnEl.style.display = 'block';
          warnEl.textContent = '⚠ 引用文件缺失（可先上传到中转站再运行）:\n' + warnings.join('\n');
          return;
        }
        closeRunModal();
        loadRuns();
      } catch (e) { document.getElementById('run-playbook-error').textContent = e.message; }
    });

    // ---- 创建/编辑向导 ----
    bindCpTaskEvents();
    document.querySelectorAll('.wiz-step').forEach(btn => {
      btn.addEventListener('click', () => showCpStep(parseInt(btn.dataset.step)));
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

    document.getElementById('cp-add-var').addEventListener('click', () => {
      cpState.vars.push({ key: '', value: '' });
      renderCpVars();
    });
    document.getElementById('cp-add-task').addEventListener('click', () => {
      const action = document.getElementById('cp-task-action').value;
      cpTaskCounter++;
      const task = { name: `任务 ${cpTaskCounter}`, action, args: getActionArgs(action) };
      cpState.tasks.push(task);
      renderCpTasks();
    });

    document.getElementById('cp-save-btn').addEventListener('click', async () => {
      const err = document.getElementById('cp-error');
      const name = document.getElementById('cp-name').value.trim();
      if (!name) { err.textContent = '剧本名称不能为空'; return; }

      const vars = {};
      for (const v of cpState.vars) {
        if (v.key.trim()) vars[v.key.trim()] = v.value;
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
        showToast('剧本已保存', 'success');
        await loadAll();
        if (state.selectedId) selectPlaybook(state.selectedId);
        loadRuns();
      } catch (e) {
        err.textContent = e.message;
      } finally {
        document.getElementById('cp-save-btn').textContent = '保存剧本';
        document.getElementById('cp-save-btn').disabled = false;
      }
    });

    // ---- 初始加载 ----
    loadAll();
    loadRuns();

    const runId = new URLSearchParams(location.search).get('run');
    if (runId) {
      api.playbookRun(runId).then(run => {
        document.getElementById('run-detail').dataset.runId = runId;
        showRunDetail(run);
        const card = document.getElementById('run-detail-card');
        if (card) card.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      }).catch(err => showRunDetailError(err));
    }

    // 离开页面时关闭 WebSocket：否则每次进入都留下一条永久重连的悬挂连接
    return () => {
      if (ws) ws.close();
      clearTimeout(searchDebounceTimer);
      clearTimeout(cancelConfirmTimer);
    };
  });
}
