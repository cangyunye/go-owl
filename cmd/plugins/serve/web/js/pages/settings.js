export function renderSettings(render, navigate, user, api) {
  loadSettings();

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  async function loadSettings() {
    try {
      const res = await api.settings();
      renderTable(res.data || []);
    } catch { renderTable([]); }
  }

  function renderTable(settings) {
    const list = document.getElementById('settings-list');
    if (settings.length === 0) {
      list.innerHTML = '<tr><td colspan="3" class="empty-state">No settings</td></tr>';
    } else {
      list.innerHTML = settings.map(s => `<tr>
        <td style="font-family:monospace;font-size:13px">${esc(s.key)}</td>
        <td style="max-width:400px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(s.value)}</td>
        <td><button class="edit-setting-btn" data-key="${esc(s.key)}" data-value="${esc(s.value)}" style="background:none;border:1px solid var(--border);color:var(--text-muted);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px">Edit</button></td>
      </tr>`).join('');
    }

    document.querySelectorAll('.edit-setting-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const key = btn.dataset.key;
        const value = btn.dataset.value;
        document.getElementById('setting-key').textContent = key;
        document.getElementById('setting-value-input').value = value;
        document.getElementById('settings-error').textContent = '';
        document.getElementById('settings-modal').classList.add('open');
      });
    });
  }

  render(`
    <div class="card">
      <table>
        <thead><tr><th>Key</th><th>Value</th><th>Actions</th></tr></thead>
        <tbody id="settings-list"><tr><td colspan="3" class="loading">Loading...</td></tr></tbody>
      </table>
    </div>
    <div style="display:flex;gap:8px">
      <button class="btn btn-primary btn-sm" id="add-setting-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 添加配置</button>
    </div>

    <div class="modal-overlay" id="settings-modal">
      <div class="modal modal-sm">
        <h3>Edit Setting: <span id="setting-key"></span></h3>
        <div class="modal-form">
          <div class="form-row"><label>Value</label><input id="setting-value-input" placeholder="value"></div>
        </div>
        <p class="error-msg" id="settings-error"></p>
        <div class="modal-actions">
          <button class="btn-cancel" id="settings-cancel">Cancel</button>
          <button class="btn-primary" id="settings-save">Save</button>
        </div>
      </div>
    </div>

    <div class="modal-overlay" id="add-setting-modal">
      <div class="modal modal-sm">
        <h3>Add Setting</h3>
        <div class="modal-form">
          <div class="form-row"><label>Key</label><input id="add-setting-key" placeholder="setting_key"></div>
          <div class="form-row"><label>Value</label><input id="add-setting-value" placeholder="value"></div>
        </div>
        <p class="error-msg" id="add-setting-error"></p>
        <div class="modal-actions">
          <button class="btn-cancel" id="add-setting-cancel">Cancel</button>
          <button class="btn-primary" id="add-setting-submit">Add</button>
        </div>
      </div>
    </div>
  `, () => {
    document.getElementById('settings-cancel').addEventListener('click', () => {
      document.getElementById('settings-modal').classList.remove('open');
    });
    document.getElementById('settings-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('settings-modal').classList.remove('open');
    });
    document.getElementById('settings-save').addEventListener('click', async () => {
      const key = document.getElementById('setting-key').textContent;
      const value = document.getElementById('setting-value-input').value.trim();
      if (!value) { document.getElementById('settings-error').textContent = 'Value required'; return; }
      try {
        await api.updateSetting(key, value);
        document.getElementById('settings-modal').classList.remove('open');
        loadSettings();
      } catch (e) { document.getElementById('settings-error').textContent = e.message; }
    });

    document.getElementById('add-setting-btn').addEventListener('click', () => {
      document.getElementById('add-setting-modal').classList.add('open');
    });
    document.getElementById('add-setting-cancel').addEventListener('click', () => {
      document.getElementById('add-setting-modal').classList.remove('open');
    });
    document.getElementById('add-setting-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) document.getElementById('add-setting-modal').classList.remove('open');
    });
    document.getElementById('add-setting-submit').addEventListener('click', async () => {
      const key = document.getElementById('add-setting-key').value.trim();
      const value = document.getElementById('add-setting-value').value.trim();
      if (!key || !value) { document.getElementById('add-setting-error').textContent = 'Key and value required'; return; }
      try {
        await api.updateSetting(key, value);
        document.getElementById('add-setting-modal').classList.remove('open');
        document.getElementById('add-setting-key').value = '';
        document.getElementById('add-setting-value').value = '';
        loadSettings();
      } catch (e) { document.getElementById('add-setting-error').textContent = e.message; }
    });
  });
}
