export function renderNodeDetail(render, navigate, user, api, nodeId) {
  const isEditing = location.search.includes('edit=1');
  const canWrite = ['admin', 'editor', 'operator'].includes(user.role);
  const isAdmin = user.role === 'admin';

  render(`
    <div class="app-header">
      <h1>OWL Console</h1>
      <div class="header-right">
        <a href="/" style="font-size:14px;color:var(--text-muted)">Nodes</a>
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
      <a href="/" class="back-link">&larr; Back to Nodes</a>
      <div id="node-content" class="loading">Loading...</div>
    </div>

    <div class="modal-overlay" id="delete-modal">
      <div class="modal modal-sm">
        <h3>Delete Node</h3>
        <p>Are you sure you want to delete <strong id="delete-name"></strong>?</p>
        <p class="error-msg" id="delete-error"></p>
        <div class="modal-actions">
          <button class="btn-cancel" id="delete-cancel">Cancel</button>
          <button class="btn-danger" id="delete-confirm">Delete</button>
        </div>
      </div>
    </div>
  `, () => {
    document.getElementById('logout-btn').addEventListener('click', () => {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      navigate('/login');
    });
    if (isAdmin) {
      document.getElementById('delete-cancel').addEventListener('click', () => {
        document.getElementById('delete-modal').classList.remove('open');
      });
      document.getElementById('delete-modal').addEventListener('click', (e) => {
        if (e.target === e.currentTarget) {
          document.getElementById('delete-modal').classList.remove('open');
        }
      });
    }
    loadNode();
  });

  async function loadNode() {
    const container = document.getElementById('node-content');
    try {
      const n = await api.node(nodeId);
      if (isEditing && canWrite) {
        renderEditForm(container, n);
      } else {
        renderDetail(container, n);
      }
    } catch (e) {
      container.innerHTML = `<p style="color:var(--danger)">${esc(e.message)}</p>`;
    }
  }

  function renderDetail(container, n) {
    const groups = (n.groups || []).map(g => `<span class="tag">${esc(g)}</span>`).join('');
    const labels = Object.entries(n.labels || {}).map(([k, v]) =>
      `<span class="tag">${esc(k)}: ${esc(v)}</span>`).join('');

    container.innerHTML = `
      <div class="page-header">
        <h2>${esc(n.name || n.id)}</h2>
        <span class="status-badge status-${n.status || 'unknown'}">${esc(n.status || 'unknown')}</span>
      </div>
      <div class="node-detail-grid">
        <div class="card">
          <div class="detail-field"><label>ID</label><div class="value">${esc(n.id)}</div></div>
          <div class="detail-field"><label>Address</label><div class="value">${esc(n.address)}:${n.port}</div></div>
          <div class="detail-field"><label>User</label><div class="value">${esc(n.user)}</div></div>
          <div class="detail-field"><label>Proxy Jump</label><div class="value">${n.proxy_jump ? esc(n.proxy_jump) : '<span style="color:var(--text-muted)">None</span>'}</div></div>
        </div>
        <div class="card">
          <div class="detail-field"><label>Groups</label><div class="value">${groups || '<span style="color:var(--text-muted)">None</span>'}</div></div>
          <div class="detail-field"><label>Labels</label><div class="value">${labels || '<span style="color:var(--text-muted)">None</span>'}</div></div>
          <div class="detail-field"><label>Created</label><div class="value">${n.created_at ? new Date(n.created_at).toLocaleString() : '-'}</div></div>
          <div class="detail-field"><label>Updated</label><div class="value">${n.updated_at ? new Date(n.updated_at).toLocaleString() : '-'}</div></div>
        </div>
      </div>
      <div style="display:flex;gap:8px">
        ${canWrite ? `<button class="btn-primary" id="edit-btn">Edit</button>` : ''}
        ${isAdmin ? `<button class="btn-danger" id="delete-btn">Delete</button>` : ''}
      </div>
    `;

    if (canWrite) {
      document.getElementById('edit-btn').addEventListener('click', () => {
        navigate(`/nodes/${encodeURIComponent(nodeId)}?edit=1`);
      });
    }
    if (isAdmin) {
      document.getElementById('delete-btn').addEventListener('click', () => {
        document.getElementById('delete-name').textContent = n.name || n.id;
        document.getElementById('delete-modal').classList.add('open');
        document.getElementById('delete-confirm').onclick = async () => {
          try {
            await api.deleteNode(nodeId);
            navigate('/');
          } catch (e) {
            document.getElementById('delete-error').textContent = e.message;
          }
        };
      });
    }
  }

  function renderEditForm(container, n) {
    container.innerHTML = `
      <div class="page-header"><h2>Edit: ${esc(n.name || n.id)}</h2></div>
      <div class="card" style="max-width:600px">
        <div class="modal-form">
          <div class="form-row"><label>Name</label><input id="edit-name" value="${esc(n.name || '')}"></div>
          <div class="form-row"><label>Address</label><input id="edit-address" value="${esc(n.address || '')}"></div>
          <div class="form-row"><label>Port</label><input id="edit-port" type="number" value="${n.port || 22}"></div>
          <div class="form-row"><label>User</label><input id="edit-user" value="${esc(n.user || '')}"></div>
          <div class="form-row"><label>Password</label><input id="edit-password" type="password" placeholder="leave blank to keep current"></div>
          <div class="form-row"><label>SSH Key</label><textarea id="edit-sshkey" rows="3" placeholder="leave blank to keep current"></textarea></div>
          <div class="form-row"><label>Status</label>
            <select id="edit-status">
              <option value="unknown" ${n.status === 'unknown' ? 'selected' : ''}>Unknown</option>
              <option value="online" ${n.status === 'online' ? 'selected' : ''}>Online</option>
              <option value="offline" ${n.status === 'offline' ? 'selected' : ''}>Offline</option>
            </select>
          </div>
          <div class="form-row"><label>Groups</label><input id="edit-groups" value="${esc((n.groups || []).join(', '))}"></div>
          <div class="form-row"><label>Labels</label><input id="edit-labels" value="${esc(Object.entries(n.labels || {}).map(([k,v]) => k+':'+v).join(', '))}"></div>
        </div>
        <p class="error-msg" id="edit-error"></p>
        <div style="display:flex;gap:8px;margin-top:16px">
          <button class="btn-cancel" id="edit-cancel">Cancel</button>
          <button class="btn-primary" id="edit-save">Save</button>
        </div>
      </div>
    `;

    document.getElementById('edit-cancel').addEventListener('click', () => {
      navigate(`/nodes/${encodeURIComponent(nodeId)}`);
    });

    document.getElementById('edit-save').addEventListener('click', async () => {
      const data = {};
      const fields = ['name', 'address', 'user', 'status'];
      fields.forEach(f => {
        const val = document.getElementById('edit-' + f).value.trim();
        if (val !== (n[f] || '')) data[f] = val;
      });

      const port = parseInt(document.getElementById('edit-port').value);
      if (port !== (n.port || 22)) data.port = port;

      const pw = document.getElementById('edit-password').value;
      if (pw) data.password = pw;

      const sshKey = document.getElementById('edit-sshkey').value;
      if (sshKey) data.ssh_key = sshKey;

      const groups = document.getElementById('edit-groups').value.split(/[,\s]+/).filter(Boolean);
      if (JSON.stringify(groups) !== JSON.stringify(n.groups || [])) data.groups = groups;

      const labelsRaw = document.getElementById('edit-labels').value.trim();
      const labels = {};
      if (labelsRaw) {
        labelsRaw.split(',').forEach(pair => {
          const [k, ...vs] = pair.split(':');
          if (k && vs.length) labels[k.trim()] = vs.join(':').trim();
        });
      }
      if (JSON.stringify(labels) !== JSON.stringify(n.labels || {})) data.labels = labels;

      if (Object.keys(data).length === 0) {
        navigate(`/nodes/${encodeURIComponent(nodeId)}`);
        return;
      }

      const btn = document.getElementById('edit-save');
      btn.disabled = true;
      btn.textContent = 'Saving...';
      try {
        await api.updateNode(nodeId, data);
        navigate(`/nodes/${encodeURIComponent(nodeId)}`);
      } catch (e) {
        document.getElementById('edit-error').textContent = e.message;
        btn.disabled = false;
        btn.textContent = 'Save';
      }
    });
  }

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
}
