export function renderPlaybooks(render, navigate, user, api) {
  loadPlaybooks();

  let ws = null;

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  async function loadSettingsPath() {
    try {
      const res = await api.playbookSettingsPath();
      if (res.value) document.getElementById('playbook-path').value = res.value;
    } catch {}
  }

  async function loadPlaybooks() {
    try {
      const res = await api.playbooks();
      renderTable(res.data || []);
    } catch { renderTable([]); }
  }

  function renderTable(playbooks) {
    const list = document.getElementById('playbook-list');
    if (playbooks.length === 0) {
      list.innerHTML = '<tr><td colspan="4" class="empty-state">No playbooks found. Set the library path and refresh.</td></tr>';
    } else {
      list.innerHTML = playbooks.map(pb => `<tr class="${pb.file_exists === false ? 'missing-file' : ''}">
        <td>${esc(pb.name)}${pb.file_exists === false ? ' <span class="missing-badge">missing</span>' : ''}</td>
        <td>${esc(pb.description || '')}</td>
        <td>${esc((pb.task_names || []).join(', '))}</td>
        <td class="action-cell">
          <button class="run-playbook-btn" data-name="${esc(pb.name)}" ${pb.file_exists === false ? 'disabled' : ''} style="background:none;border:1px solid var(--primary);color:var(--primary);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px">Run</button>
        </td>
      </tr>`).join('');
    }

    document.querySelectorAll('.run-playbook-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        document.getElementById('run-playbook-name').value = btn.dataset.name;
        document.getElementById('run-playbook-target').value = '';
        document.getElementById('run-playbook-error').textContent = '';
        document.getElementById('run-playbook-modal').style.display = 'flex';
      });
    });
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

    <div class="card">
      <div class="card-header">
        <h3>剧本库</h3>
        <button class="btn btn-ghost btn-sm" id="add-playbook-btn">+ 新建</button>
      </div>
      <div class="card-body" style="padding:0">
        <table class="data-table">
          <thead><tr><th>名称</th><th>描述</th><th>任务</th><th></th></tr></thead>
          <tbody id="playbook-list"><tr><td colspan="4" class="loading">加载中…</td></tr></tbody>
        </table>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <h3>运行历史</h3>
      </div>
      <div class="card-body" style="padding:0">
        <table class="data-table">
          <thead><tr><th>剧本</th><th>目标节点</th><th>状态</th><th>开始时间</th><th></th></tr></thead>
          <tbody id="playbook-runs-list"><tr><td colspan="5" class="loading">加载中…</td></tr></tbody>
        </table>
      </div>
    </div>

    <div class="card" id="run-detail-card">
      <div class="card-header">
        <h3>运行详情</h3>
      </div>
      <div class="card-body" id="run-detail" data-run-id=""><p class="empty-state">选择一个运行记录查看详情</p></div>
    </div>

    <div class="modal-overlay" id="run-playbook-modal">
      <div class="modal modal-sm">
        <h3>Run Playbook: <span id="run-playbook-name-display"></span></h3>
        <div class="modal-form">
          <input type="hidden" id="run-playbook-name">
          <div class="form-row"><label>Target Nodes</label><input id="run-playbook-target" placeholder="node1,node2 (comma-separated IDs)"></div>
          <div class="form-row"><label>Tags (optional)</label><input id="run-playbook-tags" placeholder="tag1,tag2"></div>
        </div>
        <p class="error-msg" id="run-playbook-error"></p>
        <div class="modal-actions">
          <button class="btn-cancel" id="run-playbook-cancel">Cancel</button>
          <button class="btn-primary" id="run-playbook-submit">Execute</button>
        </div>
      </div>
    </div>
  `, () => {
    loadSettingsPath();
    setupWebSocket();

    document.getElementById('refresh-playbooks-btn').addEventListener('click', async () => {
      const path = document.getElementById('playbook-path').value.trim();
      if (!path) { alert('Library path is required'); return; }
      document.getElementById('refresh-playbooks-btn').textContent = 'Refreshing...';
      document.getElementById('refresh-playbooks-btn').disabled = true;
      try {
        await api.refreshPlaybooks(path);
        loadPlaybooks();
      } catch (e) { alert('Refresh failed: ' + e.message); }
      document.getElementById('refresh-playbooks-btn').textContent = 'Refresh';
      document.getElementById('refresh-playbooks-btn').disabled = false;
    });

    document.getElementById('run-playbook-cancel').addEventListener('click', () => {
      document.getElementById('run-playbook-modal').style.display = 'none';
    });
    document.getElementById('run-playbook-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('run-playbook-modal').style.display = 'none';
    });
    document.getElementById('run-playbook-submit').addEventListener('click', async () => {
      const name = document.getElementById('run-playbook-name').value;
      const target = document.getElementById('run-playbook-target').value.trim();
      const tags = document.getElementById('run-playbook-tags').value.trim();
      if (!target) { document.getElementById('run-playbook-error').textContent = 'Target nodes required'; return; }
      const targetNodes = target.split(',').map(s => s.trim()).filter(Boolean);
      try {
        await api.runPlaybook(name, { target_nodes: targetNodes, tags: tags || undefined });
        document.getElementById('run-playbook-modal').style.display = 'none';
        loadRuns();
      } catch (e) { document.getElementById('run-playbook-error').textContent = e.message; }
    });
  });
}
