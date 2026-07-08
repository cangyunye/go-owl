export function renderNodeDetail(render, navigate, user, api, nodeId) {
  const canWrite = ['admin', 'editor', 'operator'].includes(user.role);
  const isAdmin = user.role === 'admin';

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  render(`
    <div style="padding:12px 0">
      <a href="/nodes" class="back-link">&larr; Back to Nodes</a>
    </div>
    <div id="node-content" class="loading">Loading...</div>

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

    <div class="modal-overlay" id="edit-modal-overlay" role="dialog" aria-modal="true" aria-label="编辑节点">
      <div class="modal">
        <h2>编辑节点</h2>
        <div class="form-row">
          <label>节点 ID</label>
          <input type="text" class="input" id="edit-id" readonly disabled style="background:var(--bg);color:var(--muted)">
        </div>
        <div class="form-row">
          <label>节点名称</label>
          <input type="text" class="input" id="edit-name" placeholder="My Server">
        </div>
        <div class="form-row">
          <label>IP 地址 *</label>
          <input type="text" class="input" id="edit-address" placeholder="10.0.0.1">
        </div>
        <div class="form-row">
          <label>端口</label>
          <input type="number" class="input" id="edit-port" value="22">
        </div>
        <div class="form-row">
          <label>SSH 用户 *</label>
          <input type="text" class="input" id="edit-user" placeholder="root">
        </div>
        <div class="form-row">
          <label>密码</label>
          <input type="password" class="input" id="edit-password" placeholder="留空则不修改">
        </div>
        <div class="form-row">
          <label>SSH 密钥</label>
          <textarea class="input" id="edit-sshkey" placeholder="留空则不修改" rows="2" style="font-family:var(--font-mono);font-size:12px"></textarea>
        </div>
        <div class="form-row">
          <label>状态</label>
          <select class="select" id="edit-status">
            <option value="unknown">未知</option>
            <option value="online">在线</option>
            <option value="offline">离线</option>
          </select>
        </div>
        <div class="form-row">
          <label>分组</label>
          <input type="text" class="input" id="edit-groups" placeholder="web, prod">
        </div>
        <div class="form-row">
          <label>标签</label>
          <input type="text" class="input" id="edit-labels" placeholder="env:prod, tier:frontend">
        </div>
        <p class="error-msg" id="edit-error"></p>
        <div class="form-actions">
          <button class="btn btn-secondary" id="edit-cancel">取消</button>
          <button class="btn btn-primary" id="edit-save">保存</button>
        </div>
      </div>
    </div>
  `, () => {
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
    document.getElementById('edit-cancel').addEventListener('click', hideEditModal);
    document.getElementById('edit-save').addEventListener('click', handleUpdate);
    document.getElementById('edit-modal-overlay').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) hideEditModal();
    });
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        const overlay = document.querySelector('.modal-overlay.open');
        if (overlay) overlay.classList.remove('open');
      }
    });
    loadNode();
  });

  let currentNode = null;

  function hideEditModal() {
    document.getElementById('edit-modal-overlay').classList.remove('open');
  }

  function showEditModal(n) {
    currentNode = n;
    document.getElementById('edit-id').value = n.id;
    document.getElementById('edit-name').value = n.name || '';
    document.getElementById('edit-address').value = n.address || '';
    document.getElementById('edit-port').value = n.port || 22;
    document.getElementById('edit-user').value = n.user || '';
    document.getElementById('edit-password').value = '';
    document.getElementById('edit-sshkey').value = '';
    document.getElementById('edit-status').value = n.status || 'unknown';
    document.getElementById('edit-groups').value = (n.groups || []).join(', ');
    document.getElementById('edit-labels').value = Object.entries(n.labels || {}).map(([k, v]) => k + ':' + v).join(', ');
    document.getElementById('edit-error').textContent = '';
    document.getElementById('edit-modal-overlay').classList.add('open');
  }

  async function handleUpdate() {
    const n = currentNode;
    if (!n) return;
    const data = {};
    const name = document.getElementById('edit-name').value.trim();
    if (name !== (n.name || '')) data.name = name;
    const address = document.getElementById('edit-address').value.trim();
    if (address !== (n.address || '')) data.address = address;
    const port = parseInt(document.getElementById('edit-port').value) || 22;
    if (port !== (n.port || 22)) data.port = port;
    const user = document.getElementById('edit-user').value.trim();
    if (user !== (n.user || '')) data.user = user;
    const pw = document.getElementById('edit-password').value;
    if (pw) data.password = pw;
    const sshKey = document.getElementById('edit-sshkey').value;
    if (sshKey) data.ssh_key = sshKey;
    const status = document.getElementById('edit-status').value;
    if (status !== (n.status || 'unknown')) data.status = status;
    const groups = document.getElementById('edit-groups').value.split(',').map(s => s.trim()).filter(Boolean);
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

    if (Object.keys(data).length === 0) { hideEditModal(); return; }

    const btn = document.getElementById('edit-save');
    btn.disabled = true; btn.textContent = '保存中…';
    try {
      await api.updateNode(nodeId, data);
      hideEditModal();
      loadNode();
    } catch (e) { document.getElementById('edit-error').textContent = e.message; }
    btn.disabled = false; btn.textContent = '保存';
  }

  async function loadNode() {
    const container = document.getElementById('node-content');
    try {
      const n = await api.node(nodeId);
      renderDetail(container, n);
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
        <span id="ping-result" style="font-size:13px;margin-left:8px"></span>
        <span id="check-result" style="font-size:13px;margin-left:8px"></span>
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
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        ${canWrite ? `<button class="btn-secondary" id="ping-btn">Ping</button>` : ''}
        ${canWrite ? `<button class="btn-secondary" id="check-btn">SSH 检查</button>` : ''}
        ${canWrite ? `<button class="btn-primary" id="edit-btn">编辑</button>` : ''}
        ${isAdmin ? `<button class="btn-danger" id="delete-btn">删除</button>` : ''}
      </div>
    `;

    if (canWrite) {
      document.getElementById('edit-btn').addEventListener('click', () => showEditModal(n));
      document.getElementById('ping-btn').addEventListener('click', async () => {
        const btn = document.getElementById('ping-btn');
        const result = document.getElementById('ping-result');
        btn.disabled = true; btn.textContent = 'Pinging…';
        result.innerHTML = '<span style="color:var(--muted)">⏳</span>';
        try {
          const res = await api.pingNodes([nodeId]);
          const r = res.results[0];
          if (r && r.success) {
            result.innerHTML = `<span style="color:var(--success)">✓ ${r.latency_ms}ms</span>`;
          } else {
            result.innerHTML = `<span style="color:var(--danger)">✗ ${r?.error || 'failed'}</span>`;
          }
        } catch (e) { result.innerHTML = `<span style="color:var(--danger)">${esc(e.message)}</span>`; }
        btn.disabled = false; btn.textContent = 'Ping';
      });
      document.getElementById('check-btn').addEventListener('click', async () => {
        const btn = document.getElementById('check-btn');
        const result = document.getElementById('check-result');
        btn.disabled = true; btn.textContent = 'Checking…';
        result.innerHTML = '<span style="color:var(--muted)">⏳</span>';
        try {
          const res = await api.checkNodes([nodeId]);
          const r = res.results[0];
          if (r && r.success) {
            result.innerHTML = `<span style="color:var(--success)">✓ SSH ${r.method}</span>`;
            loadNode();
          } else {
            result.innerHTML = `<span style="color:var(--danger)">✗ ${r?.error || 'failed'}</span>`;
          }
        } catch (e) { result.innerHTML = `<span style="color:var(--danger)">${esc(e.message)}</span>`; }
        btn.disabled = false; btn.textContent = 'SSH 检查';
      });
    }
    if (isAdmin) {
      document.getElementById('delete-btn').addEventListener('click', () => {
        document.getElementById('delete-name').textContent = n.name || n.id;
        document.getElementById('delete-modal').classList.add('open');
        document.getElementById('delete-confirm').onclick = async () => {
          try {
            await api.deleteNode(nodeId);
            navigate('/nodes');
          } catch (e) {
            document.getElementById('delete-error').textContent = e.message;
          }
        };
      });
    }
  }
}
