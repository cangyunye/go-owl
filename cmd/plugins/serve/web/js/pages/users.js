export function renderUsers(render, navigate, user, api, shell) {
  let state = { page: 1, pageSize: 20, query: '', total: 0, role: '', roleCounts: {} };
  loadUsers();

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  async function loadUsers() {
    try {
      const params = { page: state.page, page_size: state.pageSize };
      if (state.query) params.q = state.query;
      if (state.role) params.role = state.role;
      const res = await api.users(params);
      state.total = res.meta?.total || 0;
      state.roleCounts = res.meta?.role_counts || {};
      renderTable(res.data || []);
      renderRolePanel();
    } catch { state.total = 0; renderTable([]); }
  }

  function renderRolePanel() {
    const roles = [
      { key: 'viewer', label: 'viewer' },
      { key: 'editor', label: 'editor' },
      { key: 'operator', label: 'operator' },
      { key: 'admin', label: 'admin' }
    ];
    const items = [
      `<li class="panel-item ${state.role === '' ? 'active' : ''}" data-panel-role="">
        <span class="group-text">全部</span>
        <span class="count">${state.total}</span>
      </li>`
    ].concat(roles.map(r => `
      <li class="panel-item ${state.role === r.key ? 'active' : ''}" data-panel-role="${r.key}">
        <span class="role-badge role-${r.key}" style="flex-shrink:0">${r.label}</span>
        <span class="group-text" style="flex:1"></span>
        <span class="count">${state.roleCounts[r.key] || 0}</span>
      </li>`)).join('');
    shell.setPanelContent(items);
    document.querySelectorAll('#panelList [data-panel-role]').forEach(el => {
      el.addEventListener('click', () => {
        state.role = el.dataset.panelRole;
        state.page = 1;
        loadUsers();
      });
    });
  }

  function tagColor(s) { let h = 0; for (let i = 0; i < s.length; i++) h = ((h << 5) - h) + s.charCodeAt(i); return 'tag-r' + (Math.abs(h) % 12); }

  function renderTable(users) {
    const list = document.getElementById('users-list');
    if (!list) return;
    if (users.length === 0) {
      list.innerHTML = '<div class="user-empty">暂无用户，点击右上角「添加用户」创建</div>';
    } else {
      list.innerHTML = users.map(u => {
        const initial = (u.display_name || u.username || '?').trim().charAt(0).toUpperCase();
        return `<div class="user-row">
          <span class="user-avatar ${tagColor(u.username || '?')}" style="background:oklch(62% var(--tag-c) var(--tag-h));color:#fff">${esc(initial)}</span>
          <div class="user-info">
            <div class="user-name">${esc(u.username)}</div>
            <div class="user-sub">${esc(u.display_name || '未设置显示名')}</div>
          </div>
          <span class="role-badge role-${esc(u.role)}">${esc(u.role)}</span>
          <div class="user-actions">
            <button class="btn btn-ghost btn-sm edit-user-btn" data-id="${u.id}" data-username="${esc(u.username)}" data-role="${esc(u.role)}" data-display_name="${esc(u.display_name || '')}" data-node_scope="${esc(u.node_scope || '')}">编辑</button>
            <button class="btn btn-ghost btn-sm delete-user-btn" data-id="${u.id}" data-username="${esc(u.username)}" style="color:var(--danger)">删除</button>
          </div>
        </div>`;
      }).join('');
    }

    document.querySelectorAll('.edit-user-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        document.getElementById('edit-user-id').value = btn.dataset.id;
        document.getElementById('edit-username').textContent = btn.dataset.username;
        document.getElementById('edit-role').value = btn.dataset.role;
        document.getElementById('edit-display-name').value = btn.dataset.display_name;
        document.getElementById('edit-user-password').value = '';
        let scopeGroups = '', scopeNodes = '';
        try {
          const obj = btn.dataset.node_scope ? JSON.parse(btn.dataset.node_scope) : null;
          if (obj) {
            scopeGroups = (obj.groups || []).join(',');
            scopeNodes = (obj.nodes || []).join(',');
          }
        } catch {}
        document.getElementById('edit-scope-groups').value = scopeGroups;
        document.getElementById('edit-scope-nodes').value = scopeNodes;
        document.getElementById('user-edit-error').textContent = '';
        document.getElementById('user-edit-modal').classList.add('open');
      });
    });

    document.querySelectorAll('.delete-user-btn').forEach(btn => {
      btn.addEventListener('click', async () => {
        if (!confirm(`确定删除用户「${btn.dataset.username}」？此操作不可恢复。`)) return;
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
    <div class="section-card">
      <div class="panel-head">
        <div style="flex:1;min-width:0">
          <h3 class="panel-title"><svg width="15" height="15" aria-hidden="true"><use href="#icon-users"/></svg> 用户管理</h3>
          <div class="panel-desc">账号、角色与显示名称；角色决定可访问的功能范围，变更即时生效</div>
        </div>
        <div class="input" style="position:relative;padding-left:32px;width:220px">
          <svg width="14" height="14" aria-hidden="true" style="position:absolute;left:10px;top:50%;transform:translateY(-50%);color:var(--muted)"><use href="#icon-search"/></svg>
          <input type="text" id="user-search-input" placeholder="搜索用户名 / 显示名…" aria-label="搜索用户" style="border:none;background:transparent;outline:none;color:var(--fg);width:100%;font:var(--fs-sm)/1.5 var(--font-body)" value="${esc(state.query)}">
        </div>
        <button class="btn btn-primary btn-sm" id="add-user-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 添加用户</button>
      </div>
      <div class="panel-body" style="padding:4px 24px">
        <div id="users-list"><div class="user-empty">加载中…</div></div>
      </div>
      <div class="panel-foot" style="justify-content:center">
        <button class="btn btn-ghost btn-sm" id="user-prev-btn" disabled>‹</button>
        <span class="field-hint" style="padding:0 8px" id="user-page-info"></span>
        <button class="btn btn-ghost btn-sm" id="user-next-btn">›</button>
      </div>
    </div>

    <details class="section-card" id="matrix-toggle">
        <summary class="panel-head">
          <div style="flex:1;min-width:0">
            <h3 class="panel-title">权限矩阵</h3>
            <div class="panel-desc">各角色可访问的功能范围一览，点击收起 / 展开</div>
          </div>
        </summary>
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


    <div class="modal-overlay" id="user-add-modal">
      <div class="modal modal-sm">
        <h3>添加用户</h3>
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
          <div class="form-row"><label>授权分组</label><input id="add-scope-groups" placeholder="逗号分隔，如 web,db；留空=不限"></div>
          <div class="form-row"><label>授权节点</label><input id="add-scope-nodes" placeholder="节点 ID 逗号分隔（可选）"></div>
        </div>
        <p class="error-msg" id="user-add-error"></p>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="user-add-cancel">取消</button>
          <button class="btn btn-primary" id="user-add-submit">创建</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="user-edit-modal">
      <div class="modal modal-sm">
        <h3>编辑用户：<span id="edit-username"></span></h3>
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
          <div class="form-row"><label>授权分组</label><input id="edit-scope-groups" placeholder="逗号分隔；留空=不限"></div>
          <div class="form-row"><label>授权节点</label><input id="edit-scope-nodes" placeholder="节点 ID 逗号分隔（可选）"></div>
        </div>
        <p class="error-msg" id="user-edit-error"></p>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="user-edit-cancel">取消</button>
          <button class="btn btn-primary" id="user-edit-submit">保存</button>
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
    // scope 表单 → JSON（两组都空 = 不限，传空串）
    const buildScope = (groupsId, nodesId) => {
      const groups = document.getElementById(groupsId).value.split(',').map(s => s.trim()).filter(Boolean);
      const nodes = document.getElementById(nodesId).value.split(',').map(s => s.trim()).filter(Boolean);
      if (!groups.length && !nodes.length) return '';
      return JSON.stringify({ groups, nodes });
    };

    document.getElementById('user-add-submit').addEventListener('click', async () => {
      const username = document.getElementById('add-username').value.trim();
      const password = document.getElementById('add-password').value;
      const role = document.getElementById('add-role').value;
      const display_name = document.getElementById('add-display-name').value.trim();
      if (!username || !password) { document.getElementById('user-add-error').textContent = '请填写用户名和密码'; return; }
      try {
        await api.createUser({ username, password, role, display_name: display_name || undefined,
          node_scope: buildScope('add-scope-groups', 'add-scope-nodes') || undefined });
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
      const body = { role, node_scope: buildScope('edit-scope-groups', 'edit-scope-nodes') };
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
