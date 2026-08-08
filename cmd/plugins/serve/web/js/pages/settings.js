export function renderSettings(render, navigate, user, api) {
  loadSettings();

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  const KNOWN_SETTINGS = {
    staging_dir: {
      desc: '文件中转站临时地址',
      defaultValue: null
    },
    staging_min_free: {
      desc: '文件中转站最小剩余空间(GB)',
      defaultValue: '10'
    }
  };

  async function loadSettings() {
    try {
      const [res, disk] = await Promise.all([
        api.settings(),
        api.staging.disk().catch(() => null)
      ]);
      const rows = res.data || [];
      Object.keys(KNOWN_SETTINGS).forEach(key => {
        if (rows.some(s => s.key === key)) return;
        let def = KNOWN_SETTINGS[key].defaultValue;
        if (key === 'staging_dir' && disk && disk.staging_dir) def = disk.staging_dir;
        if (def !== null) rows.push({ key, value: def, _default: true });
      });
      rows.sort((a, b) => a.key.localeCompare(b.key));
      renderTable(rows);
    } catch { renderTable([]); }
  }

  function renderTable(settings) {
    const list = document.getElementById('settings-list');
    if (settings.length === 0) {
      list.innerHTML = '<tr><td colspan="4" class="empty-state">No settings</td></tr>';
    } else {
      list.innerHTML = settings.map(s => {
        const known = KNOWN_SETTINGS[s.key];
        const desc = known ? known.desc : '-';
        const valueHtml = s._default
          ? `${esc(s.value)} <span style="color:var(--text-muted);font-size:11px">(默认值)</span>`
          : esc(s.value);
        return `<tr>
        <td style="font-family:monospace;font-size:13px">${esc(s.key)}</td>
        <td style="color:var(--text-muted);font-size:12px">${esc(desc)}</td>
        <td style="max-width:400px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${valueHtml}</td>
        <td><button class="edit-setting-btn" data-key="${esc(s.key)}" data-value="${esc(s.value)}" style="background:none;border:1px solid var(--border);color:var(--text-muted);padding:2px 10px;border-radius:var(--radius);cursor:pointer;font-size:12px">Edit</button></td>
      </tr>`;
      }).join('');
    }

    document.querySelectorAll('.edit-setting-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const key = btn.dataset.key;
        const value = btn.dataset.value;
        document.getElementById('setting-key').textContent = key;
        document.getElementById('setting-value-input').value = value;
        document.getElementById('setting-desc').textContent = (KNOWN_SETTINGS[key] || {}).desc || '';
        document.getElementById('settings-error').textContent = '';
        document.getElementById('settings-modal').classList.add('open');
      });
    });
  }

  // ---- AI provider config state ----
  let activeProvider = 'anthropic';

  const PROVIDER_META = {
    anthropic: { label: 'Anthropic', storageKey: 'anthropic' },
    deepseek:  { label: 'DeepSeek',  storageKey: 'deepseek' },
    openai:    { label: 'OpenAI',    storageKey: 'openai' },
    qwen:      { label: '千问',      storageKey: 'qwen' },
    volcengine:{ label: '火山引擎',  storageKey: 'volcengine' },
    minimax:   { label: 'MiniMax',   storageKey: 'minimax' },
    mimo:      { label: '小米 MiMo', storageKey: 'mimo' },
    custom:    { label: '自定义',    storageKey: 'custom' }
  };

  const PROVIDER_DEFAULTS = {
    anthropic: {
      provider: 'anthropic', baseUrl: 'https://api.anthropic.com',
      model: 'claude-sonnet-4-20250514',
      thinking: { budget_tokens: 4096, temperature: 0.7, thinking_mode: 'extended' }
    },
    deepseek: {
      provider: 'deepseek', baseUrl: 'https://api.deepseek.com',
      model: 'deepseek-v4-flash',
      thinking: { budget_tokens: 4096, temperature: 0.6, reasoning_effort: 'high', top_p: 0.9 }
    },
    openai: {
      provider: 'openai', baseUrl: 'https://api.openai.com/v1',
      model: 'gpt-4o',
      thinking: { reasoning_effort: 'high', temperature: 0.7, max_tokens: 4096 }
    },
    qwen: {
      provider: 'qwen', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      model: 'qwen-max',
      thinking: { temperature: 0.7, max_tokens: 4096, top_p: 0.9 }
    },
    volcengine: {
      provider: 'volcengine', baseUrl: 'https://ark.cn-beijing.volces.com/api/v3',
      model: 'doubao-pro-32k',
      thinking: { temperature: 0.7, max_tokens: 4096, top_p: 0.9 }
    },
    minimax: {
      provider: 'minimax', baseUrl: 'https://api.minimax.chat/v1',
      model: 'MiniMax-Text-01',
      thinking: { temperature: 0.7, max_tokens: 4096, top_p: 0.9 }
    },
    mimo: {
      provider: 'mimo', baseUrl: 'https://api.momo.mi.com/v1',
      model: 'mimo-pro',
      thinking: { temperature: 0.7, max_tokens: 4096, top_p: 0.9 }
    },
    custom: {
      provider: 'custom', baseUrl: '',
      model: '',
      thinking: { temperature: 0.7, max_tokens: 4096, top_p: 0.9 }
    }
  };

  function buildPanelHtml(provider) {
    const d = PROVIDER_DEFAULTS[provider];
    const isCustom = provider === 'custom';

    let modelHtml;
    if (isCustom) {
      modelHtml = `<input type="text" class="ai-model-select" value="${esc(d.model)}" data-provider="${provider}" placeholder="输入模型名称" spellcheck="false" />`;
    } else {
      const opts = {
        anthropic: ['claude-sonnet-4-20250514','claude-4-20250514','claude-opus-4-20250514','claude-3-5-haiku-20241022','claude-3-opus-20240229'],
        deepseek:  ['deepseek-v4-flash','deepseek-v4-pro'],
        qwen:      ['qwen-max','qwen-plus','qwen-turbo','qwen-long','qwq-32b','qwen2.5-72b-instruct'],
        volcengine: ['doubao-pro-32k','doubao-pro-128k','doubao-lite-32k','doubao-lite-128k','doubao-1.5-pro-256k'],
        minimax:    ['MiniMax-Text-01','MiniMax-M1-7B-0824','abab7'],
        mimo:       ['mimo-pro','mimo-max','mimo-lite'],
      }[provider] || [];
      modelHtml = `<select class="ai-model-select" data-provider="${provider}">${
        opts.map(m => `<option value="${m}"${m === d.model ? ' selected' : ''}>${m}</option>`).join('')
      }</select>`;
    }

    // Thinking fields
    const thinkFields = Object.entries(d.thinking).map(([key, val]) => {
      const isBool = typeof val === 'boolean';
      const isSelect = ['thinking_mode','reasoning_effort'].includes(key);
      let inputHtml;
      if (isSelect) {
        let opts;
        if (key === 'thinking_mode') opts = ['extended','normal','disabled'];
        else opts = ['high','medium','low'];
        inputHtml = `<select class="ai-thinking-field" data-key="${key}" data-provider="${provider}">${
          opts.map(o => `<option value="${o}"${o === val ? ' selected' : ''}>${o}</option>`).join('')
        }</select>`;
      } else {
        inputHtml = `<input type="number" class="ai-thinking-field" value="${val}" step="${typeof val === 'number' && val < 1 ? '0.1' : '1'}" min="0" data-key="${key}" data-provider="${provider}" />`;
      }
      return `<div class="ai-field-group"><div class="ai-field-label">${key}</div>${inputHtml}</div>`;
    }).join('');

    const baseUrlHtml = isCustom
      ? `<div class="ai-field-row" style="margin-bottom:18px"><div class="ai-field-group"><div class="ai-field-label">Provider 名称</div><input type="text" class="ai-provider" value="custom" data-provider="${provider}" spellcheck="false" /></div><div class="ai-field-group"><div class="ai-field-label">Base URL <span class="hint">必填</span></div><input type="text" class="ai-base-url" value="${esc(d.baseUrl)}" data-provider="${provider}" placeholder="https://your-endpoint.com/v1" spellcheck="false" /></div></div>`
      : `<div class="ai-field-row"><div class="ai-field-group"><div class="ai-field-label">Provider</div><input type="text" class="ai-provider" value="${d.provider}" data-provider="${provider}" spellcheck="false" /></div><div class="ai-field-group"><div class="ai-field-label">Base URL</div><input type="text" class="ai-base-url" value="${esc(d.baseUrl)}" data-provider="${provider}" spellcheck="false" /></div></div>`;

    return `
      <div class="ai-field-group">
        <div class="ai-field-label">API Key <span class="hint">必填</span></div>
        <div class="ai-input-with-toggle">
          <input type="password" class="ai-api-key" placeholder="sk-..." value="" spellcheck="false" data-provider="${provider}" />
          <button class="ai-toggle-vis" data-od-id="toggle-key-${provider}">👁</button>
        </div>
      </div>
      ${baseUrlHtml}
      <div class="ai-field-group">
        <div class="ai-field-label">Model</div>
        <div style="display:flex;gap:6px">
          <div style="flex:1">${modelHtml}</div>
          <button class="ai-outline-btn ai-fetch-models-btn" data-provider="${provider}">🔍 查询模型</button>
        </div>
      </div>
      <hr class="ai-section-divider" />
      <div class="ai-section-title">思考强度配置</div>
      <div class="ai-field-row" style="margin-bottom:12px">${thinkFields}</div>
      <div class="ai-field-group">
        <div class="ai-json-preview">
          <pre data-json-output="${provider}">${JSON.stringify(d.thinking, null, 2)}</pre>
          <div class="ai-json-actions">
            <button class="ai-outline-btn ai-edit-json-btn" data-provider="${provider}">✏️ 编辑 JSON</button>
          </div>
        </div>
      </div>
      ${isCustom ? `<hr class="ai-section-divider" />
      <div class="ai-field-group" style="margin-bottom:18px">
        <div class="ai-field-label">API 格式</div>
        <select class="ai-format" data-provider="${provider}">
          <option value="openai">OpenAI 兼容</option>
          <option value="anthropic">Anthropic 兼容</option>
        </select>
      </div>` : ''}
      <hr class="ai-section-divider" />
      <div class="ai-test-row">
        <button class="ai-btn-test ai-test-btn" data-provider="${provider}">
          <span class="ai-spinner"></span>
          <span class="label-text">🔗 测试连接</span>
        </button>
        <span class="ai-latency-result" data-latency="${provider}"></span>
      </div>`;
  }

  function buildAllPanelsHtml() {
    return Object.keys(PROVIDER_META).map(p => `
      <div class="ai-config-panel${p === 'anthropic' ? ' active' : ''}" role="tabpanel" id="panel-${p}">
        ${buildPanelHtml(p)}
      </div>`).join('');
  }

  function buildAiConfigHtml() {
    return `
    <div class="card" style="margin-top:16px">
      <div class="card-header">
        <h3>AI 供应商配置</h3>
        <button class="ai-outline-btn" id="saveAiConfig">
          <span class="icon">💾</span> 保存 AI 配置
        </button>
      </div>
      <div class="card-body">
        <nav class="ai-tab-bar" role="tablist">
          <button class="ai-tab-btn active" role="tab" data-tab="anthropic">Anthropic <span class="badge" id="badge-anthropic">○</span></button>
          <button class="ai-tab-btn" role="tab" data-tab="deepseek">DeepSeek <span class="badge" id="badge-deepseek">○</span></button>
          <button class="ai-tab-btn" role="tab" data-tab="openai">OpenAI <span class="badge" id="badge-openai">○</span></button>
          <button class="ai-tab-btn" role="tab" data-tab="qwen">千问 <span class="badge" id="badge-qwen">○</span></button>
          <button class="ai-tab-btn" role="tab" data-tab="volcengine">火山引擎 <span class="badge" id="badge-volcengine">○</span></button>
          <button class="ai-tab-btn" role="tab" data-tab="minimax">MiniMax <span class="badge" id="badge-minimax">○</span></button>
          <button class="ai-tab-btn" role="tab" data-tab="mimo">小米 MiMo <span class="badge" id="badge-mimo">○</span></button>
          <button class="ai-tab-btn" role="tab" data-tab="custom">自定义 <span class="badge" id="badge-custom">○</span></button>
        </nav>
        ${buildAllPanelsHtml()}
      </div>
    </div>

    <!-- JSON Editor Modal -->
    <div class="ai-modal-overlay" id="aiJsonModal">
      <div class="ai-modal-dialog">
        <div class="ai-modal-header">
          <h3>✏️ JSON 配置编辑器</h3>
          <button class="ai-modal-close" id="aiJsonClose">✕</button>
        </div>
        <div class="ai-modal-body">
          <textarea id="aiJsonEditor" spellcheck="false"></textarea>
          <div class="ai-json-error" id="aiJsonError">JSON 格式错误，请检查后重试</div>
        </div>
        <div class="ai-modal-footer">
          <button class="ai-outline-btn" id="aiJsonCancel">取消</button>
          <button class="ai-outline-btn" id="aiJsonSave">💾 保存 JSON</button>
        </div>
      </div>
    </div>`;
  }

  // ---- Render ----
  render(`
    <div class="card">
      <table>
        <thead><tr><th>Key</th><th>Description</th><th>Value</th><th>Actions</th></tr></thead>
        <tbody id="settings-list"><tr><td colspan="3" class="loading">Loading...</td></tr></tbody>
      </table>
    </div>
    <div style="display:flex;gap:8px">
      <button class="btn btn-primary btn-sm" id="add-setting-btn"><svg width="14" height="14" aria-hidden="true"><use href="#icon-plus"/></svg> 添加配置</button>
    </div>

    <div class="modal-overlay" id="settings-modal">
      <div class="modal modal-sm">
        <h3>Edit Setting: <span id="setting-key"></span></h3>
        <p id="setting-desc" style="font-size:12px;color:var(--text-muted);margin:4px 0 10px"></p>
        <div class="modal-form">
          <div class="form-row"><label>Value</label><input id="setting-value-input" placeholder="value"></div>
        </div>
        <p class="error-msg" id="settings-error"></p>
        <div class="modal-actions">
          <button class="btn btn-secondary" id="settings-cancel">Cancel</button>
          <button class="btn btn-primary" id="settings-save">Save</button>
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
          <button class="btn btn-secondary" id="add-setting-cancel">Cancel</button>
          <button class="btn btn-primary" id="add-setting-submit">Add</button>
        </div>
      </div>
    </div>

    ${buildAiConfigHtml()}
  `, () => {
    // ================================================================
    // AI Provider Config Logic
    // ================================================================

    // ---- Tab switching ----
    document.querySelectorAll('.ai-tab-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const tab = btn.getAttribute('data-tab');
        document.querySelectorAll('.ai-tab-btn').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        document.querySelectorAll('.ai-config-panel').forEach(p => p.classList.remove('active'));
        const panel = document.getElementById('panel-' + tab);
        if (panel) panel.classList.add('active');
        activeProvider = tab;
      });
    });

    // ---- API Key visibility toggle ----
    document.querySelectorAll('.ai-toggle-vis').forEach(btn => {
      btn.addEventListener('click', () => {
        const input = btn.parentElement.querySelector('.ai-api-key');
        if (!input) return;
        const isPassword = input.getAttribute('type') === 'password';
        input.setAttribute('type', isPassword ? 'text' : 'password');
        btn.textContent = isPassword ? '🙈' : '👁';
      });
    });

    // ---- Thinking field → JSON preview sync ----
    function buildThinkingJson(provider) {
      const fields = document.querySelectorAll('.ai-thinking-field[data-provider="' + provider + '"]');
      const obj = {};
      fields.forEach(el => {
        const key = el.getAttribute('data-key');
        let val = el.value;
        if (val !== '' && !isNaN(val) && val.trim() !== '') {
          val = Number(val);
        }
        obj[key] = val;
      });
      return obj;
    }

    function updateJsonPreview(provider) {
      const obj = buildThinkingJson(provider);
      const pre = document.querySelector('pre[data-json-output="' + provider + '"]');
      if (pre) pre.textContent = JSON.stringify(obj, null, 2);
      updateBadge(provider);
    }

    function updateBadge(provider) {
      const badge = document.getElementById('badge-' + provider);
      if (!badge) return;
      const keyInput = document.querySelector('.ai-api-key[data-provider="' + provider + '"]');
      if (keyInput && keyInput.value.trim().length > 0) {
        badge.textContent = '●';
        badge.style.color = 'var(--success)';
      } else {
        badge.textContent = '○';
        badge.style.color = '';
      }
    }

    document.querySelectorAll('.ai-thinking-field').forEach(el => {
      el.addEventListener('input', () => updateJsonPreview(el.getAttribute('data-provider')));
      el.addEventListener('change', () => updateJsonPreview(el.getAttribute('data-provider')));
    });

    document.querySelectorAll('.ai-api-key').forEach(el => {
      el.addEventListener('input', () => updateBadge(el.getAttribute('data-provider')));
    });

    // ---- JSON Editor Modal ----
    const aiModalOverlay = document.getElementById('aiJsonModal');
    const aiJsonEditor = document.getElementById('aiJsonEditor');
    let modalTarget = null;

    document.querySelectorAll('.ai-edit-json-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const provider = btn.getAttribute('data-provider');
        modalTarget = provider;
        const pre = document.querySelector('pre[data-json-output="' + provider + '"]');
        if (pre) aiJsonEditor.value = pre.textContent;
        document.getElementById('aiJsonError').classList.remove('show');
        aiModalOverlay.classList.add('open');
        setTimeout(() => aiJsonEditor.focus(), 100);
      });
    });

    function closeAiModal() {
      aiModalOverlay.classList.remove('open');
      modalTarget = null;
      document.getElementById('aiJsonError').classList.remove('show');
    }

    document.getElementById('aiJsonClose').addEventListener('click', closeAiModal);
    document.getElementById('aiJsonCancel').addEventListener('click', closeAiModal);
    aiModalOverlay.addEventListener('click', e => {
      if (e.target === aiModalOverlay) closeAiModal();
    });
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape' && aiModalOverlay.classList.contains('open')) closeAiModal();
    });

    document.getElementById('aiJsonSave').addEventListener('click', () => {
      if (!modalTarget) return;
      try {
        const parsed = JSON.parse(aiJsonEditor.value);
        const pre = document.querySelector('pre[data-json-output="' + modalTarget + '"]');
        if (pre) pre.textContent = JSON.stringify(parsed, null, 2);
        Object.keys(parsed).forEach(key => {
          const field = document.querySelector(
            '.ai-thinking-field[data-provider="' + modalTarget + '"][data-key="' + key + '"]'
          );
          if (field) field.value = String(parsed[key]);
        });
        document.getElementById('aiJsonError').classList.remove('show');
        closeAiModal();
        showAiToast('JSON 配置已更新', 'success');
      } catch (e) {
        document.getElementById('aiJsonError').textContent = 'JSON 格式错误：' + e.message;
        document.getElementById('aiJsonError').classList.add('show');
      }
    });

    // ---- Toast helper ----
    function showAiToast(msg, type) {
      const t = document.createElement('div');
      t.className = 'toast' + (type ? ' ' + type : '');
      t.textContent = msg;
      document.body.appendChild(t);
      setTimeout(() => { t.classList.add('show'); }, 10);
      setTimeout(() => { t.classList.remove('show'); setTimeout(() => t.remove(), 300); }, 2500);
    }

    // ---- Test connection ----
    document.querySelectorAll('.ai-test-btn').forEach(btn => {
      btn.addEventListener('click', async () => {
        const provider = btn.getAttribute('data-provider');
        const latencyEl = document.querySelector('.ai-latency-result[data-latency="' + provider + '"]');
        const keyInput = document.querySelector('.ai-api-key[data-provider="' + provider + '"]');

        if (btn.classList.contains('loading')) return;

        if (!keyInput || keyInput.value.trim() === '') {
          latencyEl.textContent = '⚠️ 请先填写 API Key';
          latencyEl.className = 'ai-latency-result show error';
          return;
        }

        btn.classList.add('loading');
        latencyEl.className = 'ai-latency-result';
        latencyEl.textContent = '';

        try {
          const apiKey = keyInput.value.trim();
          const providerInput = document.querySelector('.ai-provider[data-provider="' + provider + '"]');
          const baseUrlInput = document.querySelector('.ai-base-url[data-provider="' + provider + '"]');
          const modelInput = document.querySelector('.ai-model-select[data-provider="' + provider + '"]');
          const formatInput = document.querySelector('.ai-format[data-provider="' + provider + '"]');

          let baseUrl = baseUrlInput ? baseUrlInput.value.trim() : '';
          let apiType = 'openai';
          const pName = providerInput ? providerInput.value : provider;

          if (pName === 'anthropic') {
            if (!baseUrl) baseUrl = 'https://api.anthropic.com';
            apiType = 'anthropic';
          } else if (pName === 'deepseek') {
            if (!baseUrl) baseUrl = 'https://api.deepseek.com';
          } else if (pName === 'openai') {
            if (!baseUrl) baseUrl = 'https://api.openai.com/v1';
          } else if (pName === 'custom') {
            if (!baseUrl) { showAiToast('请先输入 Base URL'); btn.classList.remove('loading'); return; }
            apiType = formatInput ? formatInput.value : 'openai';
          }

          const model = modelInput ? modelInput.value.trim() : '';
          if (!model) { showAiToast('请先选择模型'); btn.classList.remove('loading'); return; }

          const keyRes = await api.getSessionKey();
          const encryptedKey = await window.CryptoWallet.encryptApiKey(keyRes.public_key_spki, apiKey);
          const res = await api.aiTest(keyRes.session_id, encryptedKey, baseUrl, apiType, model);

          if (res.success) {
            latencyEl.textContent = '✓ 延迟: ' + res.elapsed_ms + 'ms';
            latencyEl.className = 'ai-latency-result show success';
          } else {
            latencyEl.textContent = '✗ ' + (res.error || '连接失败');
            latencyEl.className = 'ai-latency-result show error';
          }
        } catch (e) {
          latencyEl.textContent = '✗ ' + (e.message || e);
          latencyEl.className = 'ai-latency-result show error';
        } finally {
          btn.classList.remove('loading');
        }
      });
    });

    // ---- Query models ----
    document.querySelectorAll('.ai-fetch-models-btn').forEach(btn => {
      btn.addEventListener('click', async () => {
        const provider = btn.getAttribute('data-provider');
        const keyInput = document.querySelector('.ai-api-key[data-provider="' + provider + '"]');
        if (!keyInput || !keyInput.value.trim()) {
          showAiToast('请先输入 API Key'); return;
        }
        const apiKey = keyInput.value.trim();

        const providerInput = document.querySelector('.ai-provider[data-provider="' + provider + '"]');
        const baseUrlInput = document.querySelector('.ai-base-url[data-provider="' + provider + '"]');
        const formatInput = document.querySelector('.ai-format[data-provider="' + provider + '"]');

        let baseUrl = baseUrlInput ? baseUrlInput.value.trim() : '';
        let apiType = 'openai';
        const pName = providerInput ? providerInput.value : provider;

        if (pName === 'anthropic') {
          if (!baseUrl) baseUrl = 'https://api.anthropic.com';
          apiType = 'anthropic';
        } else if (pName === 'deepseek') {
          if (!baseUrl) baseUrl = 'https://api.deepseek.com';
        } else if (pName === 'openai') {
          if (!baseUrl) baseUrl = 'https://api.openai.com/v1';
        } else if (pName === 'custom') {
          if (!baseUrl) { showAiToast('请先输入 Base URL'); return; }
          apiType = formatInput ? formatInput.value : 'openai';
        }

        btn.textContent = '查询中…';
        btn.disabled = true;

        try {
          const keyRes = await api.getSessionKey();
          const encryptedKey = await window.CryptoWallet.encryptApiKey(keyRes.public_key_spki, apiKey);
          const res = await api.aiModels(keyRes.session_id, encryptedKey, baseUrl, apiType);

          const modelInput = document.querySelector('.ai-model-select[data-provider="' + provider + '"]');
          if (res.models && res.models.length > 0 && modelInput) {
            const optionHtml = '<option value="">— 选择模型 —</option>'
              + res.models.map(m => '<option value="' + esc(m.id) + '">'
                + esc(m.id) + (m.owned_by ? ' (' + esc(m.owned_by) + ')' : '')
                + '</option>').join('');
            if (modelInput.tagName === 'SELECT') {
              modelInput.innerHTML = optionHtml;
            } else {
              const select = document.createElement('select');
              select.className = modelInput.className;
              select.setAttribute('data-provider', provider);
              select.innerHTML = optionHtml;
              modelInput.replaceWith(select);
            }
            showAiToast('获取到 ' + res.models.length + ' 个模型', 'success');
          } else {
            showAiToast('未获取到模型列表', 'error');
          }
        } catch (e) {
          showAiToast('查询失败: ' + (e.message || e), 'error');
        } finally {
          btn.textContent = '🔍 查询模型';
          btn.disabled = false;
        }
      });
    });

    // ---- Save per-provider config (direct CryptoWallet, no AIStorage modification) ----
    async function saveProviderConfig(provider) {
      const keyInput = document.querySelector('.ai-api-key[data-provider="' + provider + '"]');
      const providerInput = document.querySelector('.ai-provider[data-provider="' + provider + '"]');
      const baseUrlInput = document.querySelector('.ai-base-url[data-provider="' + provider + '"]');
      const modelInput = document.querySelector('.ai-model-select[data-provider="' + provider + '"]');
      const formatInput = document.querySelector('.ai-format[data-provider="' + provider + '"]');

      const apiKey = keyInput ? keyInput.value.trim() : '';
      const providerName = providerInput ? providerInput.value.trim() : provider;
      const baseUrl = baseUrlInput ? baseUrlInput.value.trim() : '';
      const model = modelInput ? modelInput.value.trim() : '';
      const apiFormat = formatInput ? formatInput.value : 'openai';

      const namespace = (user?.id || user?.username || 'default') + '_' + provider;
      const packet = await window.CryptoWallet.encryptLocal({ apiKey, provider: providerName, model, baseUrl, apiFormat }, namespace);
      localStorage.setItem('owl_ai_key_' + provider, JSON.stringify(packet));

      // Also update the legacy owl_ai_key for the active provider so ai.js chat works
      if (provider === activeProvider && apiKey) {
        const legacyPacket = await window.CryptoWallet.encryptLocal({ apiKey, provider: providerName, model, baseUrl, apiFormat }, user?.id || user?.username || 'default');
        localStorage.setItem('owl_ai_key', JSON.stringify(legacyPacket));
      }
    }

    async function loadProviderConfig(provider) {
      const raw = localStorage.getItem('owl_ai_key_' + provider);
      if (!raw) return null;
      try {
        const packet = JSON.parse(raw);
        const namespace = (user?.id || user?.username || 'default') + '_' + provider;
        return await window.CryptoWallet.decryptLocal(packet, namespace);
      } catch { return null; }
    }

    // ---- Save AI Config ----
    document.getElementById('saveAiConfig').addEventListener('click', async () => {
      try {
        const providers = ['anthropic', 'deepseek', 'openai', 'qwen', 'volcengine', 'minimax', 'mimo', 'custom'];

        for (const p of providers) {
          const keyInput = document.querySelector('.ai-api-key[data-provider="' + p + '"]');
          if (keyInput && keyInput.value.trim()) {
            await saveProviderConfig(p);
          }
        }

        showAiToast('✅ 所有供应商配置已保存', 'success');
        updateBadge(activeProvider);
      } catch (e) {
        showAiToast('❌ 保存失败: ' + (e.message || e), 'error');
      }
    });

    // ---- Load saved AI config for each provider ----
    async function loadAiConfig(provider) {
      try {
        const keyData = await loadProviderConfig(provider);
        if (!keyData) return;

        const keyInput = document.querySelector('.ai-api-key[data-provider="' + provider + '"]');
        const providerInput = document.querySelector('.ai-provider[data-provider="' + provider + '"]');
        const baseUrlInput = document.querySelector('.ai-base-url[data-provider="' + provider + '"]');
        const modelInput = document.querySelector('.ai-model-select[data-provider="' + provider + '"]');
        const formatInput = document.querySelector('.ai-format[data-provider="' + provider + '"]');

        if (keyInput && keyData.apiKey) keyInput.value = keyData.apiKey;
        if (providerInput && keyData.provider) providerInput.value = keyData.provider;
        if (baseUrlInput && keyData.baseUrl) baseUrlInput.value = keyData.baseUrl;
        if (modelInput) modelInput.value = keyData.model || '';
        if (formatInput && keyData.apiFormat) formatInput.value = keyData.apiFormat;

        updateBadge(provider);
      } catch {}
    }

    // Load all providers
    ['anthropic', 'deepseek', 'openai', 'qwen', 'volcengine', 'minimax', 'mimo', 'custom'].forEach(loadAiConfig);

    // ---- KV Settings Modal Logic ----
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
      if (key === 'staging_min_free' && (!/^\d+$/.test(value) || Number(value) <= 0)) {
        document.getElementById('settings-error').textContent = 'staging_min_free 必须是正整数（GB）';
        return;
      }
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
