export function renderExec(render, navigate, user, api, shell) {
  let allNodes = [];
  let selectedNodes = new Set();
  let wsCleanup = null;
  let currentTaskIDs = [];

  function esc(s) { return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }

  shell.setPanelContent(`
    <li class="panel-item active"><span class="dot" style="background:var(--accent)"></span>选择节点</li>
    <li class="panel-item" style="cursor:default;padding:4px 10px;font-size:12px;color:var(--muted)">从下方节点列表中选择目标</li>
  `);

  async function loadNodes() {
    try {
      const res = await api.nodes({ page: 1, page_size: 200 });
      allNodes = res.data || [];
      renderNodeChips();
    } catch { allNodes = []; }
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

  async function handleExec() {
    const cmd = document.getElementById('cmd-input').value.trim();
    if (!cmd || selectedNodes.size === 0) return;

    const nodeIDs = Array.from(selectedNodes);
    const isSingle = nodeIDs.length === 1;
    const timeout = parseInt(document.getElementById('timeout-val').textContent) || 30;
    const parallel = parseInt(document.getElementById('parallel-val').textContent) || 5;

    clearTerminal();
    appendTerminal(`正在连连接 ${nodeIDs.length} 个节点…`, 'ts');

    try {
      const res = await api.execAdvanced({
        node_ids: nodeIDs,
        command: cmd,
        force: 'true',
      });

      const tasks = res.tasks || [];
      currentTaskIDs = tasks.map(t => t.id);

      if (isSingle && tasks.length === 1) {
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
        appendTerminal('✓ 任务已提交', 'ok');
      }
    } catch (e) {
      appendTerminal('✗ 执行失败: ' + esc(e.message || '未知错误'), 'err');
    }
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
            <div class="node-selector" id="node-chips">
              <span style="color:var(--muted);font-size:12px">加载中…</span>
            </div>
          </div>
        </div>
        <div class="card">
          <div class="card-header"><h3>执行参数</h3></div>
          <div class="card-body">
            <div class="param-group">
              <div class="param-row">
                <label>超时时间</label>
                <span class="val" id="timeout-val">30s</span>
              </div>
              <div class="param-row">
                <label>并行数</label>
                <input type="range" min="1" max="20" value="5" oninput="document.getElementById('parallel-val').textContent=this.value">
                <span class="val" id="parallel-val">5</span>
              </div>
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

    document.getElementById('cmd-input').addEventListener('input', updateExecButton);
    document.getElementById('exec-btn').addEventListener('click', handleExec);
    document.getElementById('clear-cmd-btn').addEventListener('click', () => {
      document.getElementById('cmd-input').value = '';
      updateExecButton();
    });
    document.getElementById('clear-term-btn').addEventListener('click', clearTerminal);
  });
}
