export function renderUsers(render, navigate, user, api) {
  let state = { page: 1, pageSize: 20, query: '', total: 0 };
  loadUsers();

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  async function loadUsers() {
    try {
      const params = { page: state.page, page_size: state.pageSize };
      if (state.query) params.q = state.query;
      const res = await api.users(params);
      state.total = res.meta?.total || 0;
      renderTable(res.data || []);
    } catch { state.total = 0; renderTable([]); }
  }

  function renderTable(users) {
    const list = document.getElementById('users-list');
    if (users.length === 0) {
      list.innerHTML = '<tr><td colspan="4" class="empty-state">No users</td></tr>';
    } else {
      list.innerHTML = users.map(u => `<tr>
        <td>${esc(u.username)}</td>
        <td>${esc(u.display_name || '')}</td>
        <td><span class="role-badge role-${esc(u.role)}">${esc(u.role)}</span></td>
        <td class="action-cell">
          <button class="edit-user-btn" data-id="${u.id}" data-username="${esc(u.username)}" data-role="${esc(u.role)}" data-display_name="${esc(u.display_name || '')}" style="background:none;border:1px solid var(--border);color:var(--text-muted);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px;margin-right:4px">Edit</button>
          <button class="delete-user-btn" data-id="${u.id}" data-username="${esc(u.username)}" style="background:none;border:1px solid var(--danger);color:var(--danger);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px">Delete</button>
        </td>
      </tr>`).join('');
    }

    document.querySelectorAll('.edit-user-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        document.getElementById('edit-user-id').value = btn.dataset.id;
        document.getElementById('edit-username').textContent = btn.dataset.username;
        document.getElementById('edit-role').value = btn.dataset.role;
        document.getElementById('edit-display-name').value = btn.dataset.display_name;
        document.getElementById('edit-user-password').value = '';
        document.getElementById('user-edit-error').textContent = '';
        document.getElementById('user-edit-modal').classList.add('open');
      });
    });

    document.querySelectorAll('.delete-user-btn').forEach(btn => {
      btn.addEventListener('click', async () => {
        if (!confirm(`Delete user "${btn.dataset.username}"? This cannot be undone.`)) return;
        try {
          await api.deleteUser(btn.dataset.id);
          loadUsers();
        } catch (e) { alert('Delete failed: ' + e.message); }
      });
    });

    updatePagination();
  }

  function updatePagination() {
    const totalPages = Math.max(1, Math.ceil(state.total / state.pageSize));
    const info = document.getElementById('user-page-info');
    if (info) info.textContent = `共 ${state.total} 条 · 第 ${state.page}/${totalPages} 页`;
    const prev = document.getElementById('user-prev-btn');
    const next = document.getElementById('user-next-btn');
    if (prev) prev.disabled = state.page <= 1;
    if (next) next.disabled = state.page >= totalPages;
  }

  render(`
    <div style="display:flex;gap:8px;align-items:center">
      <button class="btn btn-primary btn-sm" id="add-user-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 添加用户</button>
      <div style="flex:1"></div>
      <div class="input" style="position:relative;padding-left:32px;width:240px">
        <svg width="14" height="14" aria-hidden="true" style="position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--muted)"><use href="#icon-search"/></svg>
        <input type="text" id="user-search-input" placeholder="搜索用户名 / 显示名…" aria-label="搜索用户" style="border:none;background:transparent;outline:none;color:var(--fg);width:100%;font:13px/1.5 var(--font-body)" value="${esc(state.query)}">
      </div>
    </div>

    <details class="card matrix-card" id="matrix-toggle" open>
        <summary class="matrix-summary">Permission Matrix <span class="matrix-hint">(click to collapse)</span></summary>
        <div class="matrix-scroll">
          <table class="matrix-table">
            <thead>
              <tr>
                <th>Permission</th>
                <th class="role-col role-viewer">viewer</th>
                <th class="role-col role-editor">editor</th>
                <th class="role-col role-operator">operator</th>
                <th class="role-col role-admin">admin</th>
              </tr>
            </thead>
            <tbody>
              <tr><td>List / View nodes</td><td class="check-cell">✓</td><td class="check-cell">✓</td><td class="check-cell">✓</td><td class="check-cell">✓</td></tr>
              <tr><td>Search / Filter nodes</td><td class="check-cell">✓</td><td class="check-cell">✓</td><td class="check-cell">✓</td><td class="check-cell">✓</td></tr>
              <tr><td>Create nodes</td><td class="dash-cell">—</td><td class="check-cell">✓</td><td class="check-cell">✓</td><td class="check-cell">✓</td></tr>
              <tr><td>Update nodes</td><td class="dash-cell">—</td><td class="check-cell">✓</td><td class="check-cell">✓</td><td class="check-cell">✓</td></tr>
              <tr><td>Execute commands</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="check-cell">✓</td><td class="check-cell">✓</td></tr>
              <tr><td>Delete nodes</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="check-cell">✓</td></tr>
              <tr><td>Cancel tasks</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="check-cell">✓</td></tr>
              <tr><td>Manage settings</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="check-cell">✓</td></tr>
              <tr class="matrix-last-row"><td>Manage users</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="dash-cell">—</td><td class="check-cell">✓</td></tr>
            </tbody>
          </table>
        </div>
      </details>

      <div class="card">
        <table>
          <thead><tr><th>Username</th><th>Display Name</th><th>Role</th><th>Actions</th></tr></thead>
          <tbody id="users-list"><tr><td colspan="4" class="loading">Loading...</td></tr></tbody>
        </table>
        <div style="display:flex;justify-content:center;gap:6px;padding:4px 0">
          <button class="btn btn-ghost btn-sm" id="user-prev-btn" disabled>‹</button>
          <span style="font-size:12px;color:var(--muted);padding:0 8px" id="user-page-info"></span>
          <button class="btn btn-ghost btn-sm" id="user-next-btn">›</button>
        </div>
      </div>

    <div class="modal-overlay" id="user-add-modal">
      <div class="modal modal-sm">
        <h3>Add User</h3>
        <div class="modal-form">
          <div class="form-row"><label>Username</label><input id="add-username" placeholder="username"></div>
          <div class="form-row"><label>Display Name</label><input id="add-display-name" placeholder="display name (optional)"></div>
          <div class="form-row"><label>Password</label><input id="add-password" type="password" placeholder="password"></div>
          <div class="form-row"><label>Role</label>
            <select id="add-role">
              <option value="viewer">viewer</option>
              <option value="editor">editor</option>
              <option value="operator">operator</option>
              <option value="admin">admin</option>
            </select>
          </div>
        </div>
        <p class="error-msg" id="user-add-error"></p>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="user-add-cancel">Cancel</button>
          <button class="btn btn-primary" id="user-add-submit">Create</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="user-edit-modal">
      <div class="modal modal-sm">
        <h3>Edit User: <span id="edit-username"></span></h3>
        <div class="modal-form">
          <input type="hidden" id="edit-user-id">
          <div class="form-row"><label>Display Name</label><input id="edit-display-name" placeholder="display name"></div>
          <div class="form-row"><label>New Password</label><input id="edit-user-password" type="password" placeholder="leave blank to keep current"></div>
          <div class="form-row"><label>Role</label>
            <select id="edit-role">
              <option value="viewer">viewer</option>
              <option value="editor">editor</option>
              <option value="operator">operator</option>
              <option value="admin">admin</option>
            </select>
          </div>
        </div>
        <p class="error-msg" id="user-edit-error"></p>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="user-edit-cancel">Cancel</button>
          <button class="btn btn-primary" id="user-edit-submit">Save</button>
        </div>
      </div>
    </div>
  `, () => {
    let searchDebounceTimer;
    document.getElementById('user-search-input').addEventListener('input', (e) => {
      clearTimeout(searchDebounceTimer);
      searchDebounceTimer = setTimeout(() => {
        state.query = e.target.value.trim();
        state.page = 1;
        loadUsers();
      }, 100);
    });

    document.getElementById('user-prev-btn').addEventListener('click', () => {
      if (state.page > 1) { state.page--; loadUsers(); }
    });
    document.getElementById('user-next-btn').addEventListener('click', () => {
      const totalPages = Math.ceil(state.total / state.pageSize);
      if (state.page < totalPages) { state.page++; loadUsers(); }
    });

    document.getElementById('add-user-btn').addEventListener('click', () => {
      document.getElementById('add-username').value = '';
      document.getElementById('add-display-name').value = '';
      document.getElementById('add-password').value = '';
      document.getElementById('add-role').value = 'viewer';
      document.getElementById('user-add-error').textContent = '';
      document.getElementById('user-add-modal').classList.add('open');
    });
    document.getElementById('user-add-cancel').addEventListener('click', () => {
      document.getElementById('user-add-modal').classList.remove('open');
    });
    document.getElementById('user-add-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('user-add-modal').classList.remove('open');
    });
    document.getElementById('user-add-submit').addEventListener('click', async () => {
      const username = document.getElementById('add-username').value.trim();
      const password = document.getElementById('add-password').value;
      const role = document.getElementById('add-role').value;
      const display_name = document.getElementById('add-display-name').value.trim();
      if (!username || !password) { document.getElementById('user-add-error').textContent = 'Username and password required'; return; }
      try {
        await api.createUser({ username, password, role, display_name: display_name || undefined });
        document.getElementById('user-add-modal').classList.remove('open');
        state.page = 1;
        loadUsers();
      } catch (e) { document.getElementById('user-add-error').textContent = e.message; }
    });

    document.getElementById('user-edit-cancel').addEventListener('click', () => {
      document.getElementById('user-edit-modal').classList.remove('open');
    });
    document.getElementById('user-edit-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('user-edit-modal').classList.remove('open');
    });
    document.getElementById('user-edit-submit').addEventListener('click', async () => {
      const id = document.getElementById('edit-user-id').value;
      const role = document.getElementById('edit-role').value;
      const display_name = document.getElementById('edit-display-name').value.trim();
      const password = document.getElementById('edit-user-password').value;
      const body = { role };
      if (display_name) body.display_name = display_name;
      if (password) body.password = password;
      try {
        await api.updateUser(id, body);
        document.getElementById('user-edit-modal').classList.remove('open');
        loadUsers();
      } catch (e) { document.getElementById('user-edit-error').textContent = e.message; }
    });
  });
}
