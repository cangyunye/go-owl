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

    <div class="card" style="margin-top:16px">
      <div class="card-header"><h3>AI 助手</h3></div>
      <div class="card-body">
        <div class="settings-field" style="margin-bottom:12px">
          <label style="display:block;margin-bottom:4px;color:var(--text-muted);font-size:13px">API Key</label>
          <input type="password" id="ai-api-key" class="input" placeholder="sk-..." style="width:100%" />
        </div>
        <div class="settings-field" style="margin-bottom:12px">
          <label style="display:block;margin-bottom:4px;color:var(--text-muted);font-size:13px">Provider</label>
          <select id="ai-provider" class="input" style="width:100%">
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic</option>
            <option value="deepseek">DeepSeek</option>
            <option value="custom">Custom</option>
          </select>
        </div>
        <div class="settings-field" style="margin-bottom:12px">
          <label style="display:block;margin-bottom:4px;color:var(--text-muted);font-size:13px">Model</label>
          <div style="display:flex;gap:6px">
            <input type="text" id="ai-model" class="input" placeholder="gpt-4o" style="flex:1" />
            <select id="ai-model-select" class="input" style="flex:1;display:none"></select>
            <button id="fetch-models-btn" class="btn btn-sm" style="white-space:nowrap">查询模型</button>
          </div>
        </div>
        <div class="settings-field" id="ai-base-url-field" style="margin-bottom:12px;display:none">
          <label style="display:block;margin-bottom:4px;color:var(--text-muted);font-size:13px">Base URL</label>
          <input type="text" id="ai-base-url" class="input" placeholder="https://api.example.com/v1" style="width:100%" />
        </div>
        <div class="settings-field" id="ai-format-field" style="margin-bottom:12px;display:none">
          <label style="display:block;margin-bottom:4px;color:var(--text-muted);font-size:13px">API 格式</label>
          <select id="ai-format" class="input" style="width:100%">
            <option value="openai">OpenAI 兼容</option>
            <option value="anthropic">Anthropic 兼容</option>
          </select>
        </div>
        <div style="display:flex;align-items:center;gap:10px">
          <button id="save-ai-config" class="btn btn-primary">保存</button>
          <span id="save-ai-hint" style="font-size:12px;color:var(--success);display:none"></span>
        </div>
      </div>
    </div>
  `, () => {
    document.getElementById('ai-provider')?.addEventListener('change', () => {
      const custom = document.getElementById('ai-provider').value === 'custom';
      document.getElementById('ai-base-url-field').style.display = custom ? '' : 'none';
      document.getElementById('ai-format-field').style.display = custom ? '' : 'none';
    });

    (async () => {
      const currentUser = user;
      const keyData = await AIStorage.loadApiKey(currentUser?.id || currentUser?.username || 'default').catch(() => null);
      if (keyData) {
        if (keyData.apiKey) { document.getElementById('ai-api-key').value = keyData.apiKey; }
        if (keyData.provider) document.getElementById('ai-provider').value = keyData.provider;
        if (keyData.model) { document.getElementById('ai-model').value = keyData.model; hideModelSelect(); }
        if (keyData.baseUrl) document.getElementById('ai-base-url').value = keyData.baseUrl;
        if (keyData.apiFormat) document.getElementById('ai-format').value = keyData.apiFormat;
        if (keyData.provider === 'custom') {
          document.getElementById('ai-base-url-field').style.display = '';
          document.getElementById('ai-format-field').style.display = '';
        }
      }
    })();

    function showToast(msg) {
      const t = document.createElement('div'); t.className = 'toast'; t.textContent = msg;
      document.body.appendChild(t); setTimeout(() => t.remove(), 2000);
    }

    function showModelSelect(models) {
      const input = document.getElementById('ai-model');
      const select = document.getElementById('ai-model-select');
      select.innerHTML = '<option value="">— 选择模型 —</option>' + models.map(m => '<option value="' + m.id + '">' + m.id + (m.owned_by ? ' (' + m.owned_by + ')' : '') + '</option>').join('');
      input.style.display = 'none';
      select.style.display = '';
    }

    function hideModelSelect() {
      document.getElementById('ai-model').style.display = '';
      document.getElementById('ai-model-select').style.display = 'none';
    }

    document.getElementById('ai-model-select')?.addEventListener('change', () => {
      const sel = document.getElementById('ai-model-select');
      if (sel.value) {
        document.getElementById('ai-model').value = sel.value;
        hideModelSelect();
      }
    });

    async function getApiKey() {
      const inputVal = document.getElementById('ai-api-key').value.trim();
      if (inputVal) return inputVal;
      const userId = user?.id || user?.username || 'default';
      const keyData = await AIStorage.loadApiKey(userId).catch(() => null);
      return keyData?.apiKey || '';
    }

    document.getElementById('fetch-models-btn')?.addEventListener('click', async () => {
      const apiKey = await getApiKey();
      if (!apiKey) { showToast('请先输入 API Key'); return; }

      const provider = document.getElementById('ai-provider').value;
      let baseUrl = document.getElementById('ai-base-url').value.trim();
      let apiType = document.getElementById('ai-format').value;

      if (provider === 'custom') {
        if (!baseUrl) { showToast('请先输入 Base URL'); return; }
      } else {
        const urls = { openai: 'https://api.openai.com', deepseek: 'https://api.deepseek.com', anthropic: 'https://api.anthropic.com' };
        baseUrl = urls[provider] || '';
        apiType = provider === 'anthropic' ? 'anthropic' : 'openai';
      }

      try {
        document.getElementById('fetch-models-btn').textContent = '查询中…';
        document.getElementById('fetch-models-btn').disabled = true;

        const keyRes = await api.getSessionKey();
        const encryptedKey = await CryptoWallet.encryptApiKey(keyRes.public_key_spki, apiKey);
        const res = await api.aiModels(keyRes.session_id, encryptedKey, baseUrl, apiType);

        if (res.models && res.models.length > 0) {
          showModelSelect(res.models);
          showToast('获取到 ' + res.models.length + ' 个模型');
        } else {
          showToast('未获取到模型列表');
        }
      } catch (e) {
        showToast('查询失败: ' + (e.message || e));
      } finally {
        document.getElementById('fetch-models-btn').textContent = '查询模型';
        document.getElementById('fetch-models-btn').disabled = false;
      }
    });

    document.getElementById('save-ai-config')?.addEventListener('click', async () => {
      let key = document.getElementById('ai-api-key').value;
      const provider = document.getElementById('ai-provider').value;
      const model = document.getElementById('ai-model').value;
      const baseUrl = document.getElementById('ai-base-url').value;
      const apiFormat = document.getElementById('ai-format').value;
      const userId = user?.id || user?.username || 'default';
      if (!key) {
        const existing = await AIStorage.loadApiKey(userId).catch(() => null);
        if (existing && existing.apiKey) {
          key = existing.apiKey;
        } else {
          showToast('请先输入 API Key'); return;
        }
      }
      await AIStorage.saveApiKey(userId, key, provider, model, baseUrl, apiFormat);
      const hint = document.getElementById('save-ai-hint');
      hint.textContent = '✓ 保存成功';
      hint.style.display = '';
      setTimeout(() => { hint.style.display = 'none'; }, 3000);
    });
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
