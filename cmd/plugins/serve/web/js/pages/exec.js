export function renderExec(render, navigate, user, api, shell) {
  let allNodes = [];
  let selectedNodes = new Set();
  let wsCleanup = null;
  let currentTaskIDs = [];
  let activeGroups = [];
  let allGroups = [];
  let allLabels = [];
  let labelInputs = [];

  const params = new URLSearchParams(window.location.search);
  const initNodes = params.get('nodes');
  const initGroups = params.get('groups');
  if (initGroups) activeGroups = initGroups.split(',').filter(Boolean);
  if (initNodes) initNodes.split(',').filter(Boolean).forEach(id => selectedNodes.add(id));

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  function tagColor(s) { let h = 0; for (let i = 0; i < s.length; i++) h = ((h << 5) - h) + s.charCodeAt(i); return 'tag-r' + (Math.abs(h) % 7); }

  shell.setPanelContent(`
    <li class="panel-item active"><span class="dot" style="background:var(--accent)"></span>选择节点</li>
    <li class="panel-item" style="cursor:default;padding:4px 10px;font-size:12px;color:var(--muted)">从下方节点列表中选择目标</li>
  `);

  async function loadNodes() {
    try {
      const opts = { page: 1, page_size: 200 };
      if (activeGroups.length) opts.group = activeGroups.join(',');
      const res = await api.nodes(opts);
      allNodes = res.data || [];
      renderNodeChips();
      if (initNodes && initNodes.split(',').length) updateExecButton();
    } catch { allNodes = []; }
  }

  async function loadFilters() {
    try {
      const res = await api.filters();
      if (res.groups) allGroups = res.groups;
      if (res.labels) allLabels = res.labels;
      renderFilterControls();
    } catch {}
  }

  function renderFilterControls() {
    const container = document.getElementById('filter-controls');
    if (!container) return;
    let groupOpts = '<option value="">所有分组</option>';
    allGroups.forEach(g => {
      const sel = activeGroups.includes(g) ? ' selected' : '';
      groupOpts += `<option value="${esc(g)}"${sel}>${esc(g)}</option>`;
    });
    container.innerHTML = `
      <div class="filter-row">
        <label>分组</label>
        <select id="filter-group" class="exec-select">${groupOpts}</select>
      </div>
      <div class="filter-row">
        <label>标签</label>
        <div style="display:flex;gap:4px;flex-wrap:wrap" id="label-tags"></div>
        <div style="display:flex;gap:4px;margin-top:4px">
          <input type="text" id="label-input" class="exec-input" placeholder="key=value" style="flex:1;min-width:0">
          <button class="btn btn-ghost btn-sm" id="add-label-btn">+</button>
        </div>
      </div>
    `;

    document.getElementById('filter-group').addEventListener('change', function() {
      activeGroups = this.value ? [this.value] : [];
      const card = document.getElementById('group-filter-card');
      if (activeGroups.length) {
        card.style.display = '';
        document.getElementById('group-tags').innerHTML = activeGroups.map(g =>
          `<span class="tag ${tagColor(g)}">${esc(g)} <span class="tag-remove" data-group="${esc(g)}" style="cursor:pointer;margin-left:2px">×</span></span>`
        ).join('');
        document.querySelectorAll('.tag-remove').forEach(el => {
          el.addEventListener('click', function() {
            activeGroups = activeGroups.filter(g => g !== this.dataset.group);
            document.getElementById('filter-group').value = '';
            if (activeGroups.length === 0) {
              document.getElementById('group-filter-card').style.display = 'none';
            } else {
              document.getElementById('group-tags').innerHTML = activeGroups.map(g =>
                `<span class="tag ${tagColor(g)}">${esc(g)}</span>`
              ).join('');
            }
            loadNodes();
          });
        });
      } else {
        card.style.display = 'none';
      }
      loadNodes();
    });

    document.getElementById('add-label-btn').addEventListener('click', addLabel);
    document.getElementById('label-input').addEventListener('keydown', e => { if (e.key === 'Enter') addLabel(); });

    renderLabelTags();
  }

  function addLabel() {
    const input = document.getElementById('label-input');
    const val = input.value.trim();
    if (!val || !val.includes('=')) return;
    if (!labelInputs.includes(val)) {
      labelInputs.push(val);
      input.value = '';
      renderLabelTags();
    }
  }

  function removeLabel(l) {
    labelInputs = labelInputs.filter(x => x !== l);
    renderLabelTags();
  }

  function renderLabelTags() {
    const container = document.getElementById('label-tags');
    if (!container) return;
    container.innerHTML = labelInputs.map(l =>
      `<span class="tag tag-blue">${esc(l)} <span class="label-remove" data-label="${esc(l)}" style="cursor:pointer;margin-left:2px">×</span></span>`
    ).join('');
    document.querySelectorAll('.label-remove').forEach(el => {
      el.addEventListener('click', function() { removeLabel(this.dataset.label); });
    });
  }

  function renderNodeChips() {
    const container = document.getElementById('node-chips');
    if (!container) return;
    if (allNodes.length === 0) {
      container.innerHTML = '<span style="color:var(--muted);font-size:12px">无可用节点</span>';
      return;
    }
    container.innerHTML = allNodes.map(n => {
      const selected = selectedNodes.has(n.id);
      const dotColor = n.status === 'online' ? 'var(--success)' : n.status === 'offline' ? 'var(--muted)' : 'var(--warn)';
      return `<span class="node-chip ${selected ? 'selected' : ''}" data-id="${esc(n.id)}" data-status="${esc(n.status)}">
        <span class="dot" style="background:${dotColor}"></span>${esc(n.name || n.id)}
      </span>`;
    }).join('');

    container.querySelectorAll('.node-chip').forEach(chip => {
      chip.addEventListener('click', () => {
        const id = chip.dataset.id;
        if (selectedNodes.has(id)) {
          selectedNodes.delete(id);
          chip.classList.remove('selected');
        } else {
          selectedNodes.add(id);
          chip.classList.add('selected');
        }
        updateExecButton();
      });
    });
  }

  function updateExecButton() {
    const btn = document.getElementById('exec-btn');
    if (!btn) return;
    const count = selectedNodes.size;
    const cmd = document.getElementById('cmd-input').value.trim();
    btn.disabled = count === 0 || !cmd;
    btn.textContent = count === 0 ? '选择目标节点' : count === 1 ? '执行命令' : `在 ${count} 个节点上执行`;
  }

  function appendTerminal(html, cls) {
    const body = document.getElementById('term-body');
    if (!body) return;
    body.innerHTML = body.innerHTML.replace('<div class="line cursor-blink"></div>', '');
    const line = document.createElement('div');
    line.className = 'line';
    if (cls) line.innerHTML = `<span class="${cls}">${html}</span>`;
    else line.innerHTML = html;
    body.appendChild(line);
    body.innerHTML += '<div class="line cursor-blink"></div>';
    body.scrollTop = body.scrollHeight;
  }

  function clearTerminal() {
    const body = document.getElementById('term-body');
    if (!body) return;
    body.innerHTML = '<div class="line cursor-blink"></div>';
  }

  function buildExecPayload() {
    const nodeIDs = Array.from(selectedNodes);
    const cmd = document.getElementById('cmd-input').value.trim();
    const formatEl = document.getElementById('format-select');
    const isAsync = document.getElementById('async-toggle')?.checked || false;
    const isSerial = document.getElementById('mode-serial')?.checked || false;
    const isDebug = document.getElementById('debug-toggle')?.checked || false;
    const retryCount = parseInt(document.getElementById('retry-count')?.value) || 3;
    const retryInterval = document.getElementById('retry-interval')?.value || '1';
    const retryMaxInterval = document.getElementById('retry-max-interval')?.value || '30';
    const noRetry = document.getElementById('no-retry')?.checked || false;
    const connectTimeout = document.getElementById('connect-timeout')?.value || '';
    const commandTimeout = document.getElementById('command-timeout')?.value || '';

    const payload = {
      node_ids: nodeIDs,
      command: cmd,
      force: 'true',
    };

    if (isAsync) payload.async = true;
    if (formatEl && formatEl.value !== 'simple') payload.format = formatEl.value;
    if (isSerial) payload.serial = true;
    if (isDebug) payload.debug = true;
    if (retryCount !== 3) payload.retry = retryCount;
    if (retryInterval !== '1') payload.retry_interval = retryInterval + 's';
    if (retryMaxInterval !== '30') payload.retry_max_interval = retryMaxInterval + 's';
    if (noRetry) payload.no_retry = true;
    if (connectTimeout) payload.connect_timeout = connectTimeout + 's';
    if (commandTimeout) payload.command_timeout = commandTimeout + 's';

    if (activeGroups.length) payload.group = activeGroups.join(',');
    if (labelInputs.length) {
      payload.labels = {};
      labelInputs.forEach(l => {
        const idx = l.indexOf('=');
        if (idx > 0) payload.labels[l.substring(0, idx)] = l.substring(idx + 1);
      });
    }

    return payload;
  }

  async function handleExec() {
    const cmd = document.getElementById('cmd-input').value.trim();
    if (!cmd || selectedNodes.size === 0) return;

    const nodeIDs = Array.from(selectedNodes);
    const isSingle = nodeIDs.length === 1;
    const isAsync = document.getElementById('async-toggle')?.checked || false;

    clearTerminal();
    const modeLabel = isAsync ? '[异步]' : '';
    appendTerminal(`${modeLabel}正在连接 ${nodeIDs.length} 个节点…`, 'ts');

    try {
      const payload = buildExecPayload();
      const res = await api.execAdvanced(payload);

      const tasks = res.tasks || [];
      currentTaskIDs = tasks.map(t => t.id);

      if (isSingle && tasks.length === 1 && !isAsync) {
        appendTerminal(`任务已创建: ${esc(tasks[0].id)}`, 'ok');
        appendTerminal('等待实时输出…', 'ts');

        if (wsCleanup) wsCleanup.close();
        wsCleanup = api.connectWebSocket(msg => {
          if (msg.type === 'task_output' && msg.data.task_id === tasks[0].id) {
            appendTerminal(esc(msg.data.line), 'out');
          } else if (msg.type === 'task_update' && msg.data.id === tasks[0].id) {
            const status = msg.data.status;
            if (status === 'completed') {
              appendTerminal('✓ 执行完成', 'ok');
            } else if (status === 'failed' || status === 'cancelled') {
              appendTerminal('✗ 执行失败: ' + esc(msg.data.error || ''), 'err');
            }
            if (wsCleanup) wsCleanup.close();
          }
        });
      } else {
        tasks.forEach(t => {
          appendTerminal(`[${esc(t.node_id)}] 任务: ${esc(t.id)}`, 'out');
        });
        const count = tasks.length;
        appendTerminal(`✓ 已提交 ${count} 个任务${isAsync ? ' (异步模式)' : ''}`, 'ok');
      }
    } catch (e) {
      appendTerminal('✗ 执行失败: ' + esc(e.message || '未知错误'), 'err');
    }
  }

  function toggleAdv(id) {
    const el = document.getElementById(id);
    if (el) el.classList.toggle('open');
    const arrow = el?.previousElementSibling?.querySelector('.arrow');
    if (arrow) arrow.classList.toggle('open');
  }

  render(`
    <div class="exec-layout">
      <div class="exec-main">
        <div class="cmd-editor">
          <div class="editor-header">
            <span>命令输入</span>
            <span style="display:flex;gap:6px">
              <button class="btn btn-ghost btn-sm" id="clear-cmd-btn">清空</button>
            </span>
          </div>
          <textarea id="cmd-input" placeholder="# 输入要执行的命令，支持多行&#10;uptime&#10;df -h&#10;free -m" aria-label="命令输入框" spellcheck="false">uptime
df -h
free -m</textarea>
        </div>

        <div class="output-terminal">
          <div class="term-header">
            <div class="dot-group">
              <span class="td td-red"></span>
              <span class="td td-yellow"></span>
              <span class="td td-green"></span>
            </div>
            <span class="term-title">输出</span>
            <span style="flex:1"></span>
            <button class="btn btn-ghost btn-sm" id="clear-term-btn">清屏</button>
          </div>
          <div class="term-body" id="term-body">
            <div class="line" style="color:var(--muted)">选择节点并点击「执行」查看输出</div>
            <div class="line cursor-blink"></div>
          </div>
        </div>
      </div>

      <div class="exec-sidebar">
        <div class="card">
          <div class="card-header"><h3>目标节点</h3></div>
          <div class="card-body">
            <div class="node-selector" id="node-chips" style="margin-bottom:8px">
              <span style="color:var(--muted);font-size:12px">加载中…</span>
            </div>
            <div class="status-filter">
              <button class="status-btn active" data-status="">全部</button>
              <button class="status-btn" data-status="online">在线</button>
              <button class="status-btn" data-status="offline">离线</button>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3>筛选条件</h3></div>
          <div class="card-body" id="filter-controls"></div>
        </div>

        <div class="card" id="group-filter-card" style="${activeGroups.length ? '' : 'display:none'}">
          <div class="card-header"><h3>活跃分组</h3></div>
          <div class="card-body" id="group-tags" style="display:flex;gap:4px;flex-wrap:wrap">
            ${activeGroups.map(g => `<span class="tag ${tagColor(g)}">${esc(g)}</span>`).join('')}
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3>执行模式</h3></div>
          <div class="card-body">
            <div class="btn-group" style="display:flex;gap:4px;margin-bottom:8px">
              <button class="btn btn-sm mode-btn active" id="mode-parallel" data-mode="parallel">并行</button>
              <button class="btn btn-sm mode-btn" id="mode-serial" data-mode="serial">串行</button>
            </div>
            <label class="toggle-row">
              <input type="checkbox" id="async-toggle">
              <span class="toggle-track"><span class="toggle-thumb"></span></span>
              <span style="font-size:12px;color:var(--muted)">异步执行</span>
            </label>
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3>输出选项</h3></div>
          <div class="card-body">
            <div class="filter-row">
              <label>格式</label>
              <select id="format-select" class="exec-select">
                <option value="simple">simple</option>
                <option value="detail">detail</option>
                <option value="json">json</option>
              </select>
            </div>
            <label class="toggle-row" style="margin-top:6px">
              <input type="checkbox" id="debug-toggle">
              <span class="toggle-track"><span class="toggle-thumb"></span></span>
              <span style="font-size:12px;color:var(--muted)">调试模式</span>
            </label>
          </div>
        </div>

        <div class="card">
          <div class="card-header"><h3>超时设置</h3></div>
          <div class="card-body">
            <div class="param-group">
              <div class="param-row">
                <label>连接超时</label>
                <div style="display:flex;gap:4px;align-items:center">
                  <input type="number" id="connect-timeout" class="exec-input" value="10" min="1" style="width:50px;text-align:center">
                  <span style="font-size:11px;color:var(--muted)">s</span>
                </div>
              </div>
              <div class="param-row">
                <label>命令超时</label>
                <div style="display:flex;gap:4px;align-items:center">
                  <input type="number" id="command-timeout" class="exec-input" value="30" min="1" style="width:50px;text-align:center">
                  <span style="font-size:11px;color:var(--muted)">s</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="adv-toggle" onclick="document.getElementById('retry-options').classList.toggle('open');this.querySelector('.arrow').classList.toggle('open')">
            <span class="arrow">▶</span> 重试设置
          </div>
          <div class="adv-options open" id="retry-options">
            <div class="adv-option-group">
              <label>最大重试次数</label>
              <input type="number" id="retry-count" value="3" min="0" style="width:60px">
            </div>
            <div class="adv-option-group">
              <label>重试间隔</label>
              <div style="display:flex;gap:4px;align-items:center">
                <input type="number" id="retry-interval" value="1" min="1" style="width:50px">
                <span style="font-size:10px;color:var(--muted)">s</span>
              </div>
            </div>
            <div class="adv-option-group">
              <label>最大间隔</label>
              <div style="display:flex;gap:4px;align-items:center">
                <input type="number" id="retry-max-interval" value="30" min="1" style="width:50px">
                <span style="font-size:10px;color:var(--muted)">s</span>
              </div>
            </div>
            <div class="adv-option-group">
              <label class="force-check" style="padding-top:14px">
                <input type="checkbox" id="no-retry"> 禁用重试
              </label>
            </div>
          </div>
        </div>

        <button class="btn btn-primary" id="exec-btn" style="width:100%;justify-content:center;padding:10px" disabled>
          <svg width="16" height="16" aria-hidden="true"><use href="#icon-play"/></svg>
          选择目标节点
        </button>
      </div>
    </div>
  `, () => {
    loadNodes();
    loadFilters();

    document.getElementById('cmd-input').addEventListener('input', updateExecButton);
    document.getElementById('exec-btn').addEventListener('click', handleExec);
    document.getElementById('clear-cmd-btn').addEventListener('click', () => {
      document.getElementById('cmd-input').value = '';
      updateExecButton();
    });
    document.getElementById('clear-term-btn').addEventListener('click', clearTerminal);

    document.querySelectorAll('.status-btn').forEach(btn => {
      btn.addEventListener('click', function() {
        document.querySelectorAll('.status-btn').forEach(b => b.classList.remove('active'));
        this.classList.add('active');
      });
    });

    document.querySelectorAll('.mode-btn').forEach(btn => {
      btn.addEventListener('click', function() {
        document.querySelectorAll('.mode-btn').forEach(b => b.classList.remove('active'));
        this.classList.add('active');
      });
    });

    const noRetryCb = document.getElementById('no-retry');
    if (noRetryCb) {
      noRetryCb.addEventListener('change', function() {
        document.getElementById('retry-count').disabled = this.checked;
        document.getElementById('retry-interval').disabled = this.checked;
        document.getElementById('retry-max-interval').disabled = this.checked;
      });
    }
  });
}
