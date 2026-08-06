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
          <button class="btn btn-secondary" id="run-playbook-cancel">Cancel</button>
          <button class="btn btn-primary" id="run-playbook-submit">Execute</button>
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
          <button class="btn btn-secondary" id="detail-cancel-btn">关闭</button>
          <button class="btn btn-primary" id="detail-run-btn">运行</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="create-playbook-modal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>📝 创建剧本</h3>
          <button class="btn btn-ghost btn-sm" id="create-pb-close-btn" style="margin-left:auto;background:none;border:none;color:var(--muted);cursor:pointer;font-size:18px">&times;</button>
        </div>
        <div class="modal-body" id="create-pb-body" style="max-height:70vh;overflow-y:auto">
          <!-- Step indicator -->
          <div style="display:flex;gap:8px;margin-bottom:16px" id="create-pb-steps">
            <span class="step-dot active" data-step="1">1</span><span style="color:var(--muted);font-size:12px">基本信息</span>
            <span style="color:var(--muted);padding:0 4px">→</span>
            <span class="step-dot" data-step="2">2</span><span style="color:var(--muted);font-size:12px">变量</span>
            <span style="color:var(--muted);padding:0 4px">→</span>
            <span class="step-dot" data-step="3">3</span><span style="color:var(--muted);font-size:12px">执行配置</span>
            <span style="color:var(--muted);padding:0 4px">→</span>
            <span class="step-dot" data-step="4">4</span><span style="color:var(--muted);font-size:12px">任务</span>
            <span style="color:var(--muted);padding:0 4px">→</span>
            <span class="step-dot" data-step="5">5</span><span style="color:var(--muted);font-size:12px">确认</span>
          </div>

          <!-- Step 1: Basic Info -->
          <div class="create-pb-page" data-page="1">
            <div class="form-row"><label>剧本名称 *</label><input id="cp-name" placeholder="my-playbook" style="width:100%"></div>
            <div class="form-row"><label>描述</label><textarea id="cp-desc" placeholder="可选" style="width:100%;resize:vertical" rows="2"></textarea></div>
            <div class="form-row"><label>版本</label><input id="cp-version" value="1.0" style="width:100%"></div>
          </div>

          <!-- Step 2: Variables -->
          <div class="create-pb-page" data-page="2" style="display:none">
            <p style="font-size:13px;color:var(--muted);margin-bottom:12px">添加变量（可选）</p>
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
            <h4 style="font-size:13px;color:var(--muted);margin:16px 0 8px">默认配置（可选）</h4>
            <div class="form-row"><label>目标分组</label><input id="cp-groups" placeholder="web, db (逗号分隔)" style="width:100%"></div>
            <div class="form-row"><label>执行标签</label><input id="cp-tags" placeholder="tag1, tag2 (逗号分隔)" style="width:100%"></div>
            <div class="form-row"><label>跳过标签</label><input id="cp-skip-tags" placeholder="skip-me (逗号分隔)" style="width:100%"></div>
          </div>

          <!-- Step 4: Tasks -->
          <div class="create-pb-page" data-page="4" style="display:none">
            <p style="font-size:13px;color:var(--muted);margin-bottom:12px">添加任务项</p>
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
              <p class="empty-state" style="font-size:13px;padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>
            </div>
          </div>

          <!-- Step 5: Confirm -->
          <div class="create-pb-page" data-page="5" style="display:none">
            <div id="cp-summary" style="font-size:13px;margin-bottom:12px"></div>
            <h4 style="font-size:13px;color:var(--muted);margin-bottom:8px">YAML 预览</h4>
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

    // Create playbook wizard state
    let cpState = {
      step: 1,
      totalSteps: 5,
      vars: [],
      tasks: [],
    };
    let cpTaskCounter = 0;

    document.getElementById('add-playbook-btn').addEventListener('click', () => {
      cpState = { step: 1, totalSteps: 5, vars: [], tasks: [] };
      cpTaskCounter = 0;
      document.getElementById('cp-name').value = '';
      document.getElementById('cp-desc').value = '';
      document.getElementById('cp-version').value = '1.0';
      document.getElementById('cp-mode').value = '';
      document.getElementById('cp-groups').value = '';
      document.getElementById('cp-tags').value = '';
      document.getElementById('cp-skip-tags').value = '';
      document.getElementById('cp-vars-list').innerHTML = '';
      document.getElementById('cp-skip-vars').checked = false;
      document.getElementById('cp-tasks-list').innerHTML = '<p class="empty-state" style="font-size:13px;padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>';
      document.getElementById('cp-error').textContent = '';
      showCpStep(1);
      document.getElementById('create-playbook-modal').classList.add('open');
    });

    document.getElementById('create-pb-close-btn').addEventListener('click', () => {
      document.getElementById('create-playbook-modal').classList.remove('open');
    });
    document.getElementById('create-playbook-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('create-playbook-modal').classList.remove('open');
    });

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
      const idx = cpState.vars.length;
      cpState.vars.push({ key: '', value: '' });
      const row = document.createElement('div');
      row.className = 'form-row';
      row.style.cssText = 'display:flex;gap:8px;align-items:end';
      row.innerHTML = `
        <div style="flex:1"><input class="cp-var-key" data-idx="${idx}" placeholder="变量名" style="width:100%"></div>
        <div style="flex:1"><input class="cp-var-value" data-idx="${idx}" placeholder="值" style="width:100%"></div>
        <button class="cp-var-remove" data-idx="${idx}" style="background:none;border:1px solid var(--danger);color:var(--danger);border-radius:var(--radius);cursor:pointer;padding:4px 8px;font-size:12px">删除</button>
      `;
      document.getElementById('cp-vars-list').appendChild(row);
      row.querySelector('.cp-var-key').addEventListener('input', (e) => { cpState.vars[idx].key = e.target.value; });
      row.querySelector('.cp-var-value').addEventListener('input', (e) => { cpState.vars[idx].value = e.target.value; });
      row.querySelector('.cp-var-remove').addEventListener('click', (e) => {
        const removeIdx = Array.from(document.getElementById('cp-vars-list').children).indexOf(e.currentTarget.parentElement);
        cpState.vars.splice(removeIdx, 1);
        e.currentTarget.parentElement.remove();
      });
    });

    // Tasks
    document.getElementById('cp-add-task').addEventListener('click', () => {
      const action = document.getElementById('cp-task-action').value;
      cpTaskCounter++;
      const task = { name: `任务 ${cpTaskCounter}`, action, args: getActionArgs(action) };
      cpState.tasks.push(task);
      renderCpTasks();
    });

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
      if (cpState.tasks.length === 0) {
        list.innerHTML = '<p class="empty-state" style="font-size:13px;padding:16px;text-align:center;color:var(--muted)">暂无任务，请添加</p>';
        return;
      }
      list.innerHTML = cpState.tasks.map((t, i) =>
        `<div style="display:flex;align-items:center;gap:8px;padding:8px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:6px">
          <span style="font-weight:500;font-size:13px;flex:1">${esc(t.name)}</span>
          <span class="tag">${esc(t.action)}</span>
          <button class="cp-task-remove" data-idx="${i}" style="background:none;border:1px solid var(--danger);color:var(--danger);border-radius:var(--radius);cursor:pointer;padding:2px 8px;font-size:11px">删除</button>
        </div>`
      ).join('');
      document.querySelectorAll('.cp-task-remove').forEach(btn => {
        btn.addEventListener('click', () => {
          cpState.tasks.splice(parseInt(btn.dataset.idx), 1);
          renderCpTasks();
        });
      });
    }

    function buildCpSummary() {
      const name = document.getElementById('cp-name').value.trim();
      const desc = document.getElementById('cp-desc').value.trim();
      const version = document.getElementById('cp-version').value.trim() || '1.0';
      const mode = document.getElementById('cp-mode').value || 'fail_continue';

      const html = `
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px">
          <div><strong>名称:</strong> ${esc(name)}</div>
          <div><strong>版本:</strong> ${esc(version)}</div>
          <div><strong>描述:</strong> ${esc(desc || '-')}</div>
          <div><strong>执行模式:</strong> ${mode}</div>
        </div>
      `;
      document.getElementById('cp-summary').innerHTML = html;

      // Build preview YAML (for visual only — the actual YAML is generated server-side)
      let preview = `name: ${name}\n`;
      if (desc) preview += `description: ${desc}\n`;
      preview += `version: "${version}"\nhosts: []\n`;
      if (mode) preview += `execution_mode: ${mode}\n`;
      preview += `pre_tasks: []\n`;
      preview += `tasks:\n`;
      for (const t of cpState.tasks) {
        preview += `  - name: ${t.name}\n    action: ${t.action}\n    args:\n`;
        for (const [k, v] of Object.entries(t.args)) {
          const val = typeof v === 'string' ? v : JSON.stringify(v);
          preview += `      ${k}: ${val}\n`;
        }
      }
      preview += `post_tasks: []\n`;
      document.getElementById('cp-preview').textContent = preview;
    }

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
        version: document.getElementById('cp-version').value.trim() || '1.0',
        execution_mode: document.getElementById('cp-mode').value || undefined,
        vars: Object.keys(vars).length > 0 ? vars : undefined,
        default_groups: document.getElementById('cp-groups').value.trim() ? document.getElementById('cp-groups').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        default_tags: document.getElementById('cp-tags').value.trim() ? document.getElementById('cp-tags').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        default_skip_tags: document.getElementById('cp-skip-tags').value.trim() ? document.getElementById('cp-skip-tags').value.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        tasks: cpState.tasks,
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
  });
}
