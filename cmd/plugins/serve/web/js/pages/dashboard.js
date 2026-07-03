export function renderDashboard(render, navigate, user, api) {
  let state = { nodes: [], total: 0, page: 1, pageSize: 20, query: '', filters: { groups: [], users: [] } };
  const canWrite = ['admin', 'editor', 'operator'].includes(user.role);

  async function loadFilters() {
    try { state.filters = await api.filters(); } catch {}
  }

  async function loadNodes() {
    const params = { page: state.page, page_size: state.pageSize };
    if (state.query) params.q = state.query;
    try {
      const res = state.query ? await api.searchNodes(state.query) : await api.nodes(params);
      state.nodes = (res.data || []);
      state.total = state.query ? state.nodes.length : (res.meta?.total || 0);
    } catch { state.nodes = []; state.total = 0; }
    renderTable();
  }

  function statusClass(s) { return 'status-badge status-' + (s || 'unknown'); }
  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  function renderTable() {
    const list = document.getElementById('node-list');
    if (state.nodes.length === 0) {
      list.innerHTML = '<tr><td colspan="6" class="empty-state">No nodes found</td></tr>';
    } else {
      list.innerHTML = state.nodes.map(n => {
        const groups = (n.groups || []).map(g => `<span class="tag">${esc(g)}</span>`).join('');
        return `<tr>
          <td class="node-name"><a href="/nodes/${esc(n.id)}">${esc(n.name || n.id)}</a></td>
          <td>${esc(n.address)}:${n.port}</td>
          <td>${esc(n.user)}</td>
          <td><span class="${statusClass(n.status)}">${esc(n.status || 'unknown')}</span></td>
          <td>${groups}</td>
          ${canWrite ? `<td><a href="/nodes/${esc(n.id)}?edit=1" style="font-size:13px;color:var(--text-muted)">Edit</a></td>` : ''}
        </tr>`;
      }).join('');
    }
    const totalPages = Math.max(1, Math.ceil(state.total / state.pageSize));
    document.getElementById('page-info').textContent = `Page ${state.page} of ${totalPages} (${state.total} total)`;
    document.getElementById('prev-btn').disabled = state.page <= 1;
    document.getElementById('next-btn').disabled = state.page >= totalPages;
  }

  function showAddModal() {
    const overlay = document.getElementById('modal-overlay');
    overlay.style.display = 'flex';
  }

  function hideAddModal() {
    document.getElementById('modal-overlay').style.display = 'none';
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
    btn.textContent = 'Creating...';
    try {
      await api.createNode(data);
      hideAddModal();
      state.page = 1;
      await loadNodes();
    } catch (e) {
      document.getElementById('add-error').textContent = e.message;
    }
    btn.disabled = false;
    btn.textContent = 'Create';
  }

  loadFilters().then(loadNodes);

  render(`
    <div class="app-header">
      <h1>OWL Console</h1>
      <div class="header-right">
        <a href="/" style="font-size:14px;color:var(--text)">Nodes</a>
        <a href="/tasks" style="font-size:14px;color:var(--text-muted)">Tasks</a>
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
        <h2>Nodes</h2>
        ${canWrite ? '<button class="btn-primary" id="add-node-btn">+ Add Node</button>' : ''}
      </div>
      <div class="search-bar">
        <input type="text" id="search-input" placeholder="Search nodes by name, address, user, or label..." value="${esc(state.query)}">
        <button id="search-btn">Search</button>
      </div>
      <div class="card">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Address</th>
              <th>User</th>
              <th>Status</th>
              <th>Groups</th>
              ${canWrite ? '<th>Actions</th>' : ''}
            </tr>
          </thead>
          <tbody id="node-list">
            <tr><td colspan="6" class="loading">Loading...</td></tr>
          </tbody>
        </table>
      </div>
      <div class="pagination">
        <button id="prev-btn">Previous</button>
        <span class="page-info" id="page-info"></span>
        <button id="next-btn">Next</button>
      </div>
    </div>

    <div class="modal-overlay" id="modal-overlay">
      <div class="modal">
        <h3>Add Node</h3>
        <div class="modal-form">
          <div class="form-row"><label>ID *</label><input id="add-id" placeholder="unique-id"></div>
          <div class="form-row"><label>Name</label><input id="add-name" placeholder="My Server"></div>
          <div class="form-row"><label>Address *</label><input id="add-address" placeholder="10.0.0.1"></div>
          <div class="form-row"><label>Port</label><input id="add-port" type="number" value="22"></div>
          <div class="form-row"><label>User *</label><input id="add-user" placeholder="root"></div>
          <div class="form-row"><label>Password</label><input id="add-password" type="password" placeholder="optional"></div>
          <div class="form-row"><label>SSH Key</label><textarea id="add-sshkey" placeholder="optional ssh private key" rows="2"></textarea></div>
          <div class="form-row"><label>Status</label>
            <select id="add-status"><option value="unknown">Unknown</option><option value="online">Online</option><option value="offline">Offline</option></select>
          </div>
          <div class="form-row"><label>Groups</label><input id="add-groups" placeholder="web, prod"></div>
          <div class="form-row"><label>Labels</label><input id="add-labels" placeholder="env:prod, tier:frontend"></div>
        </div>
        <p class="error-msg" id="add-error"></p>
        <div class="modal-actions">
          <button class="btn-cancel" id="add-cancel">Cancel</button>
          <button class="btn-primary" id="add-submit">Create</button>
        </div>
      </div>
    </div>
  `, () => {
    document.getElementById('logout-btn').addEventListener('click', () => {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      navigate('/login');
    });

    if (canWrite) {
      document.getElementById('add-node-btn').addEventListener('click', showAddModal);
      document.getElementById('add-cancel').addEventListener('click', hideAddModal);
      document.getElementById('add-submit').addEventListener('click', handleAdd);
      document.getElementById('modal-overlay').addEventListener('click', (e) => {
        if (e.target === e.currentTarget) hideAddModal();
      });
    }

    document.getElementById('search-btn').addEventListener('click', () => {
      state.query = document.getElementById('search-input').value.trim();
      state.page = 1;
      loadNodes();
    });

    document.getElementById('search-input').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        state.query = e.target.value.trim();
        state.page = 1;
        loadNodes();
      }
    });

    document.getElementById('prev-btn').addEventListener('click', () => {
      if (state.page > 1) { state.page--; loadNodes(); }
    });

    document.getElementById('next-btn').addEventListener('click', () => {
      const totalPages = Math.ceil(state.total / state.pageSize);
      if (state.page < totalPages) { state.page++; loadNodes(); }
    });
  });
}
