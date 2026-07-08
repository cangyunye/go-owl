export function renderPlaybooks(render, navigate, user, api, shell) {
  let state = {
    playbooks: [],
    filteredPlaybooks: [],
    query: '',
    selectedCategory: '',
    categories: [],
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
        `<tr class="playbook-row" data-id="${esc(pb.id)}" style="cursor:pointer">
          <td>${esc(pb.name)}${pb.file_exists === false ? ' <span class="missing-badge">missing</span>' : ''}</td>
          <td>${pb.category ? `<span class="tag">${esc(pb.category)}</span>` : '<span style="color:var(--muted);font-size:12px">-</span>'}</td>
          <td>${esc(pb.description || '')}</td>
          <td>${esc((pb.task_names || []).join(', '))}</td>
          <td class="action-cell">
            <button class="run-playbook-btn" data-id="${esc(pb.id)}" ${pb.file_exists === false ? 'disabled' : ''} style="background:none;border:1px solid var(--primary);color:var(--primary);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px">Run</button>
          </td>
        </tr>`
      ).join('');
    }
    document.querySelectorAll('.run-playbook-btn').forEach(btn => {
      btn.addEventListener('click', (e) => { e.stopPropagation(); showRunModal(btn.dataset.id); });
    });
    document.querySelectorAll('.playbook-row').forEach(row => {
      row.addEventListener('click', () => showPlaybookDetail(row.dataset.id));
    });
  }

  function showRunModal(id) {
    const pb = state.playbooks.find(p => p.id === id);
    document.getElementById('run-playbook-id').value = id;
    document.getElementById('run-playbook-name-display').textContent = pb ? pb.name : id;
    document.getElementById('run-playbook-target').value = '';
    document.getElementById('run-playbook-groups').value = '';
    document.getElementById('run-playbook-tags').value = '';
    document.getElementById('run-playbook-vars').value = '';
    document.getElementById('run-playbook-error').textContent = '';
    document.getElementById('run-playbook-modal').classList.add('open');
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
        tasks.innerHTML = `<h4 style="font-size:13px;color:var(--muted);margin-bottom:8px">任务列表 (${pb.task_names.length})</h4>
          <div style="display:flex;flex-wrap:wrap;gap:6px">${pb.task_names.map(t => `<span class="tag">${esc(t)}</span>`).join('')}</div>`;
      } else {
        tasks.innerHTML = '<p style="color:var(--muted);font-size:13px">无任务</p>';
      }
      yaml.textContent = pb.file_exists ? `文件路径: ${pb.file_path}\n\n(YAML 源文件内容暂不可用)` : '文件不存在';
    } catch {
      nameEl.textContent = '加载失败';
      meta.innerHTML = '<span style="color:var(--danger)">无法获取剧本详情</span>';
    }
  }

  function renderRuns(runs) {
    const list = document.getElementById('playbook-runs-list');
    if (!runs || runs.length === 0) {
      list.innerHTML = '<tr><td colspan="5" class="empty-state">No runs yet</td></tr>';
    } else {
      list.innerHTML = runs.map(r => `<tr>
        <td>${esc(r.playbook_name)}</td>
        <td>${esc((r.target_nodes || []).join(', '))}</td>
        <td><span class="status-badge status-${esc(r.status)}">${esc(r.status)}</span></td>
        <td style="font-size:12px;color:var(--text-muted)">${r.created_at ? new Date(r.created_at).toLocaleString() : ''}</td>
        <td class="action-cell">
          <button class="view-run-btn" data-id="${r.id}" style="background:none;border:1px solid var(--border);color:var(--text-muted);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px">View</button>
          ${r.status === 'running' || r.status === 'pending' ? `<button class="cancel-run-btn" data-id="${r.id}" style="background:none;border:1px solid var(--danger);color:var(--danger);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px;margin-left:4px">Cancel</button>` : ''}
        </td>
      </tr>`).join('');
    }

    document.querySelectorAll('.view-run-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const id = btn.dataset.id;
        api.playbookRun(id).then(run => showRunDetail(run)).catch(() => {});
      });
    });

    document.querySelectorAll('.cancel-run-btn').forEach(btn => {
      btn.addEventListener('click', async () => {
        if (!confirm('Cancel this playbook run?')) return;
        try {
          await api.cancelPlaybookRun(btn.dataset.id);
          loadRuns();
        } catch (e) { alert('Cancel failed: ' + e.message); }
      });
    });
  }

  function showRunDetail(run) {
    const detail = document.getElementById('run-detail');
    const steps = (run.results || []).map(r => `<tr>
      <td>${esc(r.task_name)}</td>
      <td>${esc(r.node_id)}</td>
      <td>${esc(r.action || '')}</td>
      <td><span class="status-badge status-${esc(r.status)}">${esc(r.status)}</span></td>
      <td>${r.exit_code !== undefined ? r.exit_code : ''}</td>
      <td style="font-size:12px;max-width:200px;overflow:hidden;text-overflow:ellipsis">${esc(r.output || '')}</td>
    </tr>`).join('');

    detail.innerHTML = `
      <div style="margin-bottom:12px;display:flex;gap:16px;flex-wrap:wrap;font-size:13px">
        <span>Playbook: <strong>${esc(run.playbook_name)}</strong></span>
        <span>Status: <span class="status-badge status-${esc(run.status)}">${esc(run.status)}</span></span>
        <span>Nodes: ${esc((run.target_nodes || []).join(', '))}</span>
        <span>Started: ${run.created_at ? new Date(run.created_at).toLocaleString() : ''}</span>
        ${run.error ? `<span style="color:var(--danger)">Error: ${esc(run.error)}</span>` : ''}
      </div>
      <div style="max-height:300px;overflow:auto">
        <table style="font-size:12px">
          <thead><tr><th>Task</th><th>Node</th><th>Action</th><th>Status</th><th>Exit</th><th>Output</th></tr></thead>
          <tbody>${steps || '<tr><td colspan="6" class="empty-state">No results</td></tr>'}</tbody>
        </table>
      </div>
    `;
  }

  async function loadRuns() {
    try {
      const res = await api.playbookRuns();
      renderRuns(res.data || []);
    } catch { renderRuns([]); }
  }

  function setupWebSocket() {
    ws = api.connectWebSocket(msg => {
      if (msg.type === 'playbook_run_update') {
        loadRuns();
        if (msg.data && msg.data.id === document.getElementById('run-detail').dataset.runId) {
          showRunDetail(msg.data);
        }
      }
    });
  }

  render(`
    <div class="card" style="margin-bottom:0">
      <div class="path-bar">
        <label for="playbook-path">Library Path</label>
        <input id="playbook-path" placeholder="/path/to/playbooks">
        <button class="btn btn-secondary btn-sm" id="refresh-playbooks-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg> 刷新</button>
      </div>
    </div>

    <div class="filter-bar">
      <div class="input" style="position:relative;padding-left:32px;width:240px">
        <svg width="14" height="14" aria-hidden="true" style="position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--muted)"><use href="#icon-search"/></svg>
        <input type="text" id="playbook-search" placeholder="搜索剧本名称 / 描述…" style="border:none;background:transparent;outline:none;color:var(--fg);width:100%;font:13px/1.5 var(--font-body)">
      </div>
      <div class="spacer"></div>
      <button class="btn btn-primary btn-sm" id="add-playbook-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 新建</button>
    </div>

    <div class="card" style="overflow:auto">
      <table class="data-table">
        <thead><tr><th>名称</th><th>分类</th><th>描述</th><th>任务</th><th></th></tr></thead>
        <tbody id="playbook-list"><tr><td colspan="5" class="loading">加载中…</td></tr></tbody>
      </table>
    </div>

    <div class="card">
      <div class="card-header"><h3>运行历史</h3></div>
      <div class="card-body" style="padding:0">
        <table class="data-table">
          <thead><tr><th>剧本</th><th>目标节点</th><th>状态</th><th>开始时间</th><th></th></tr></thead>
          <tbody id="playbook-runs-list"><tr><td colspan="5" class="loading">加载中…</td></tr></tbody>
        </table>
      </div>
    </div>

    <div class="card" id="run-detail-card">
      <div class="card-header"><h3>运行详情</h3></div>
      <div class="card-body" id="run-detail" data-run-id=""><p class="empty-state">选择一个运行记录查看详情</p></div>
    </div>

    <div class="modal-overlay" id="run-playbook-modal">
      <div class="modal modal-sm">
        <h3>Run Playbook: <span id="run-playbook-name-display"></span></h3>
        <div class="modal-form">
          <input type="hidden" id="run-playbook-id">
          <div class="form-row"><label>Target Nodes</label><input id="run-playbook-target" placeholder="node1,node2 (comma-separated IDs)"></div>
          <div class="form-row"><label>Groups (alternative)</label><input id="run-playbook-groups" placeholder="web, db (selects all nodes in these groups)"></div>
          <div class="form-row"><label>Tags (optional)</label><input id="run-playbook-tags" placeholder="tag1,tag2"></div>
          <div class="form-row"><label>Extra Vars (optional)</label><input id="run-playbook-vars" placeholder="key=value, version=2.0"></div>
        </div>
        <p class="error-msg" id="run-playbook-error"></p>
        <div class="modal-actions">
          <button class="btn-cancel" id="run-playbook-cancel">Cancel</button>
          <button class="btn-primary" id="run-playbook-submit">Execute</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="playbook-detail-modal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3 id="detail-name"></h3>
          <button class="btn btn-ghost btn-sm" id="detail-close-btn" style="margin-left:auto;background:none;border:none;color:var(--muted);cursor:pointer;font-size:18px">&times;</button>
        </div>
        <div class="modal-body" id="detail-body" style="max-height:70vh;overflow-y:auto">
          <div id="detail-meta" style="display:flex;gap:16px;flex-wrap:wrap;font-size:13px;margin-bottom:12px;padding-bottom:12px;border-bottom:1px solid var(--border)"></div>
          <div id="detail-tasks" style="margin-bottom:12px"></div>
          <div style="margin-top:12px">
            <h4 style="font-size:13px;color:var(--muted);margin-bottom:8px">YAML 源文件</h4>
            <pre id="detail-yaml" style="background:var(--code-bg);border:1px solid var(--border);border-radius:var(--radius);padding:12px;font:12px/1.6 var(--font-mono);overflow-x:auto;white-space:pre;max-height:300px"></pre>
          </div>
        </div>
        <div class="modal-actions">
          <button class="btn-cancel" id="detail-cancel-btn">关闭</button>
          <button class="btn-primary" id="detail-run-btn">运行</button>
        </div>
      </div>
    </div>
  `, () => {
    setupWebSocket();

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
      document.getElementById('refresh-playbooks-btn').textContent = 'Refreshing...';
      document.getElementById('refresh-playbooks-btn').disabled = true;
      try {
        const res = await api.refreshPlaybooks(path);
        if (res.errors && res.errors.length > 0) {
          alert('Sync completed with errors:\n' + res.errors.join('\n'));
        }
        loadAll();
      } catch (e) { alert('Refresh failed: ' + e.message); }
      document.getElementById('refresh-playbooks-btn').textContent = '刷新';
      document.getElementById('refresh-playbooks-btn').disabled = false;
    });

    document.getElementById('run-playbook-cancel').addEventListener('click', () => {
      document.getElementById('run-playbook-modal').classList.remove('open');
    });
    document.getElementById('run-playbook-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('run-playbook-modal').classList.remove('open');
    });
    document.getElementById('run-playbook-submit').addEventListener('click', async () => {
      const id = document.getElementById('run-playbook-id').value;
      const target = document.getElementById('run-playbook-target').value.trim();
      const groups = document.getElementById('run-playbook-groups').value.trim();
      const tags = document.getElementById('run-playbook-tags').value.trim();
      const varsStr = document.getElementById('run-playbook-vars').value.trim();
      if (!target && !groups) { document.getElementById('run-playbook-error').textContent = 'Target nodes or groups required'; return; }
      const body = {};
      if (target) body.target_nodes = target.split(',').map(s => s.trim()).filter(Boolean);
      if (groups) body.groups = groups.split(',').map(s => s.trim()).filter(Boolean);
      if (tags) body.tags = tags;
      if (varsStr) {
        const extraVars = {};
        varsStr.split(',').forEach(pair => {
          const [k, ...vs] = pair.split('=');
          if (k && vs.length) extraVars[k.trim()] = vs.join('=').trim();
        });
        if (Object.keys(extraVars).length) body.extra_vars = extraVars;
      }
      try {
        await api.runPlaybook(id, body);
        document.getElementById('run-playbook-modal').classList.remove('open');
        loadRuns();
      } catch (e) { document.getElementById('run-playbook-error').textContent = e.message; }
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

    loadAll();
  });
}
