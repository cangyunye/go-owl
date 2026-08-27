// 告警页：告警列表（分级/筛选/分页）、处置（确认/解决）、详情与推荐对策，
// 以及监控配置（静默/告警类型/通知渠道/对策库，admin）。
export function renderAlerts(render, navigate, user, api, shell) {
  const isOperator = ['operator', 'admin'].includes(user.role);
  const isAdmin = user.role === 'admin';
  const pageSize = 20;
  const state = {
    status: 'active', severity: '', page: 1, total: 0, items: [],
    detail: null, remedies: [], types: [],
  };
  let mode = 'list'; // list | config

  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - (t * 1000)) / 1000); if (s < 60) return s + '秒前'; if (s < 3600) return Math.floor(s / 60) + '分钟前'; if (s < 86400) return Math.floor(s / 3600) + '小时前'; return Math.floor(s / 86400) + '天前'; }
  function fmtTime(t) { return t ? new Date(t * 1000).toLocaleString('zh-CN', { hour12: false }) : '-'; }

  const STATUS_TEXT = { open: '待处理', acked: '已确认', resolved: '已解决' };
  const STATUS_CLS = { open: 'pending', acked: 'info', resolved: 'success' };
  const SEV_TEXT = { critical: '紧急', warn: '警告', info: '提示' };
  const SEV_COLOR = { critical: 'var(--danger)', warn: 'var(--warn)', info: 'var(--info)' };

  function severityBadge(s) {
    const color = SEV_COLOR[s] || 'var(--muted)';
    return `<span style="display:inline-block;padding:2px 8px;border-radius:999px;font-size:11px;font-weight:600;background:color-mix(in oklch, ${color} 15%, var(--surface));color:${color}">${SEV_TEXT[s] || esc(s)}</span>`;
  }

  function statusBadge(s) {
    return `<span class="hi-status ${STATUS_CLS[s] || 'pending'}">${STATUS_TEXT[s] || esc(s)}</span>`;
  }

  function buildParams() {
    return { status: state.status, severity: state.severity, limit: pageSize, offset: (state.page - 1) * pageSize };
  }

  function renderPanel() {
    const filters = [
      { key: 'active', label: '活跃告警' }, { key: 'open', label: '待处理' },
      { key: 'acked', label: '已确认' }, { key: 'resolved', label: '已解决' }, { key: '', label: '全部' },
    ];
    const sevs = [{ key: '', label: '全部级别' }, { key: 'critical', label: '紧急' }, { key: 'warn', label: '警告' }, { key: 'info', label: '提示' }];
    shell.setPanelTitle('告警过滤');
    shell.setPanelContent(`
      <div style="padding:12px;font-size:11px;color:var(--muted)">状态</div>
      <ul style="list-style:none;margin:0;padding:0">
        ${filters.map(f => `<li class="panel-item ${state.status === f.key ? 'active' : ''}" data-status="${f.key}" style="cursor:pointer"><span class="dot" style="background:var(--accent)"></span>${f.label}</li>`).join('')}
      </ul>
      <div style="padding:12px 12px 4px;font-size:11px;color:var(--muted)">级别</div>
      <ul style="list-style:none;margin:0;padding:0">
        ${sevs.map(f => `<li class="panel-item ${state.severity === f.key ? 'active' : ''}" data-sev="${f.key}" style="cursor:pointer"><span class="dot" style="background:${SEV_COLOR[f.key] || 'var(--muted)'}"></span>${f.label}</li>`).join('')}
      </ul>
    `);
    document.querySelectorAll('#panelList [data-status]').forEach(el => {
      el.addEventListener('click', () => { state.status = el.dataset.status; state.page = 1; load(); });
    });
    document.querySelectorAll('#panelList [data-sev]').forEach(el => {
      el.addEventListener('click', () => { state.severity = el.dataset.sev; state.page = 1; load(); });
    });
  }

  function renderView() {
    render(`
      <div class="view-head">
        <h2 class="view-title">告警中心</h2>
        <div class="view-actions" id="alert-actions" style="display:flex;gap:8px;align-items:center">
          ${isAdmin ? `<button class="btn btn-ghost btn-sm ${mode === 'config' ? 'active' : ''}" id="toggle-config"><svg width="14" height="14" aria-hidden="true"><use href="#icon-settings"/></svg> 监控配置</button>` : ''}
          <button class="btn btn-secondary btn-sm" id="alert-refresh"><svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg> 刷新</button>
          <span id="alert-count" style="font-size:12px;color:var(--muted)"></span>
        </div>
      </div>
      ${mode === 'list' ? `
      <div class="filter-row" style="margin-bottom:12px">
        <label>节点</label><input id="alert-node" placeholder="按节点 ID 过滤" style="width:160px" value="${esc(state.nodeId || '')}">
        <button class="btn btn-secondary btn-sm" id="alert-search">查询</button>
      </div>
      <div id="alert-list"></div>
      <div id="alert-pagination" style="display:flex;justify-content:center;gap:4px;margin-top:14px"></div>
      ` : `<div id="monitor-config"></div>`}
    `, afterRender);
    document.getElementById('toggle-config')?.addEventListener('click', () => {
      mode = mode === 'list' ? 'config' : 'list';
      renderView();
    });
    if (mode === 'list') {
      document.getElementById('alert-search')?.addEventListener('click', () => { state.nodeId = document.getElementById('alert-node').value.trim(); state.page = 1; load(); });
      document.getElementById('alert-refresh')?.addEventListener('click', () => load());
      document.getElementById('alert-node')?.addEventListener('keydown', e => { if (e.key === 'Enter') document.getElementById('alert-search').click(); });
      load();
    } else {
      renderConfig();
    }
  }

  function afterRender() { return null; }

  function renderList() {
    const list = document.getElementById('alert-list');
    if (!list) return;
    if (!state.items.length) {
      list.innerHTML = '<div class="view-empty" style="padding:40px"><div class="empty-title">暂无告警</div><div style="font-size:12px;color:var(--muted);margin-top:6px">监控采集每 60 秒执行一次，阈值规则见告警类型设置</div></div>';
    } else {
      list.innerHTML = `<ul class="history-list">${state.items.map(a => `
        <li class="history-item" data-alert="${esc(a.id)}" style="cursor:pointer">
          <div class="hi-icon ${a.severity === 'critical' ? 'fail' : a.severity === 'warn' ? 'pending' : 'success'}">
            <svg width="16" height="16" aria-hidden="true"><use href="#icon-bell"/></svg>
          </div>
          <div class="hi-info">
            <div class="hi-name">${esc(a.message || a.alert_type_name)}</div>
            <div class="hi-meta">${esc(a.id)} · ${esc(a.alert_type_name)} · ${esc(a.node_name || a.node_id)} · ${timeAgo(a.first_seen)}</div>
          </div>
          ${severityBadge(a.severity)}
          ${statusBadge(a.status)}
          ${isOperator && a.status !== 'resolved' ? `
            <div class="hi-action" style="display:flex;gap:4px">
              ${a.status === 'open' ? `<button class="btn btn-secondary btn-sm" data-act="ack">确认</button>` : ''}
              <button class="btn btn-ghost btn-sm" data-act="resolve">解决</button>
            </div>` : ''}
        </li>`).join('')}</ul>`;
      list.querySelectorAll('.history-item[data-alert]').forEach(el => {
        el.addEventListener('click', (e) => {
          if (e.target.closest('[data-act]')) return;
          openDetail(el.dataset.alert);
        });
      });
      list.querySelectorAll('[data-act]').forEach(btn => {
        btn.addEventListener('click', async (e) => {
          e.stopPropagation();
          const id = btn.closest('.history-item').dataset.alert;
          const act = btn.dataset.act;
          try {
            if (act === 'ack') await api.alertAck(id);
            else await api.alertResolve(id);
            load();
          } catch (err) { alert('操作失败: ' + (err.message || err)); }
        });
      });
    }
    const totalPages = Math.max(1, Math.ceil(state.total / pageSize));
    const info = document.getElementById('alert-count');
    if (info) info.textContent = `共 ${state.total} 条 · 第 ${state.page}/${totalPages} 页`;
    renderPagination();
  }

  function renderPagination() {
    const container = document.getElementById('alert-pagination');
    if (!container) return;
    const totalPages = Math.max(1, Math.ceil(state.total / pageSize));
    if (totalPages <= 1) { container.innerHTML = ''; return; }
    let html = `<button class="page-btn" data-page="${state.page - 1}" ${state.page <= 1 ? 'disabled' : ''}>◀</button>`;
    for (let i = 1; i <= totalPages; i++) {
      if (totalPages > 7 && i > 2 && i < totalPages - 1 && Math.abs(i - state.page) > 1) {
        if (html.indexOf('⋯') === -1 && i < state.page) html += `<span class="page-ellipsis">⋯</span>`;
        continue;
      }
      html += `<button class="page-btn ${i === state.page ? 'active' : ''}" data-page="${i}">${i}</button>`;
    }
    html += `<button class="page-btn" data-page="${state.page + 1}" ${state.page >= totalPages ? 'disabled' : ''}>▶</button>`;
    container.innerHTML = html;
    container.querySelectorAll('.page-btn:not(:disabled)').forEach(btn => {
      btn.addEventListener('click', () => {
        const p = parseInt(btn.dataset.page);
        if (p && p !== state.page) { state.page = p; load(); }
      });
    });
  }

  async function load() {
    renderPanel();
    try {
      const res = await api.alerts(buildParams());
      state.items = res.items || [];
      state.total = res.total || 0;
    } catch { state.items = []; state.total = 0; }
    renderList();
  }

  async function openDetail(id) {
    let rec;
    try { rec = await api.alert(id); } catch (err) { alert('加载详情失败: ' + (err.message || err)); return; }
    const a = rec.alert || {};
    state.detail = a;
    state.remedies = rec.remedies || [];
    state.types = state.types.length ? state.types : (await api.alertTypes().catch(() => ({ items: [] }))).items || [];

    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay open';
    const snapshot = (() => { try { return Object.entries(JSON.parse(a.metric_snapshot || '{}')); } catch { return []; } })();

    function planProgressHtml(plan) {
      const RUN_TEXT = { pending: '待执行', running: '执行中', waiting_approval: '待审批', done: '已完成', stopped: '已停止', failed: '失败' };
      const STEP_TEXT = { pending: '等待', running: '执行中', success: '成功', failed: '失败', skipped: '跳过', pending_approval: '待审批' };
      const STEP_COLOR = { pending: 'var(--muted)', running: 'var(--info)', success: 'var(--success)', failed: 'var(--danger)', skipped: 'var(--muted)', pending_approval: 'var(--warn)' };
      const steps = (plan.steps || []).map(st => `
        <li style="display:flex;gap:8px;align-items:flex-start;padding:8px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:6px;background:var(--bg)">
          <span style="min-width:18px;text-align:center;font-weight:600;color:var(--muted)">${st.order + 1}</span>
          <div style="flex:1;min-width:0">
            <div style="font-size:12px;font-weight:600">${esc(st.name)} <span style="color:var(--muted);font-weight:400">· ${esc(st.kind)}</span></div>
            ${st.output ? `<pre style="margin:4px 0 0;padding:6px;background:var(--surface);border-radius:var(--radius);font-family:var(--font-mono);font-size:11px;white-space:pre-wrap;word-break:break-all;max-height:120px;overflow:auto">${esc(st.output)}</pre>` : ''}
          </div>
          <span style="font-size:11px;font-weight:600;color:${STEP_COLOR[st.status] || 'var(--muted)'}">${STEP_TEXT[st.status] || esc(st.status)}</span>
        </li>`).join('');
      return `<div style="border:1px solid var(--border);border-radius:var(--radius);padding:12px;margin:12px 0;background:var(--surface)">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
          <strong style="font-size:13px">处置计划 <code>${esc(plan.id)}</code></strong>
          <span style="display:flex;gap:6px;align-items:center">
            <span style="font-size:12px;font-weight:600;color:${plan.status === 'done' ? 'var(--success)' : plan.status === 'failed' || plan.status === 'stopped' ? 'var(--danger)' : plan.status === 'waiting_approval' ? 'var(--warn)' : 'var(--info)'}">${RUN_TEXT[plan.status] || esc(plan.status)}</span>
            ${plan.status === 'running' || plan.status === 'pending' ? `<button class="btn btn-ghost btn-sm" data-stop-plan="${esc(plan.id)}">停止</button>` : ''}
            ${plan.status === 'waiting_approval' ? `
              <button class="btn btn-secondary btn-sm" data-approve-plan="${esc(plan.id)}">批准执行</button>
              <button class="btn btn-ghost btn-sm" data-reject-plan="${esc(plan.id)}">拒绝</button>` : ''}
          </span>
        </div>
        <ul style="list-style:none;margin:0;padding:0">${steps}</ul>
      </div>`;
    }

    async function loadPlanProgress(planId, area) {
      let plan = null;
      try {
        const res = await api.remedyPlan(planId);
        plan = res.run;
      } catch { return; }
      area.innerHTML = planProgressHtml(plan);
      area.querySelector('[data-stop-plan]')?.addEventListener('click', async (e) => {
        e.stopPropagation();
        try { await api.stopRemedyPlan(planId); } catch (err) { alert('停止失败: ' + (err.message || err)); }
      });
      area.querySelector('[data-approve-plan]')?.addEventListener('click', async (e) => {
        e.stopPropagation();
        try { await api.approveRemedyPlan(planId); } catch (err) { alert('批准失败: ' + (err.message || err)); }
        loadPlanProgress(planId, area);
      });
      area.querySelector('[data-reject-plan]')?.addEventListener('click', async (e) => {
        e.stopPropagation();
        if (!confirm('拒绝后该步骤不会执行，确定？')) return;
        try { await api.rejectRemedyPlan(planId); } catch (err) { alert('拒绝失败: ' + (err.message || err)); }
        loadPlanProgress(planId, area);
      });
      if (plan && !['done', 'stopped', 'failed'].includes(plan.status)) {
        setTimeout(() => loadPlanProgress(planId, area), 1500);
      }
    }

    async function executePlan() {
      const rows = [...overlay.querySelectorAll('#rm-list .rm-row')];
      const selected = rows.filter(r => r.querySelector('.rm-check').checked);
      if (!selected.length) { alert('请先勾选要执行的对策'); return; }
      const remedyIds = selected.map(r => r.dataset.rid);
      const stopOnError = overlay.querySelector('#rm-stop-on-error')?.checked !== false;
      const btn = overlay.querySelector('#rm-exec');
      btn.disabled = true;
      btn.textContent = '执行中...';
      try {
        const res = await api.createRemedyPlan(a.id, { remedy_ids: remedyIds, stop_on_error: stopOnError });
        const planArea = overlay.querySelector('#plan-area');
        planArea.innerHTML = '<p style="color:var(--muted);font-size:12px">计划已创建，执行中...</p>';
        loadPlanProgress(res.run.id, planArea);
        loadPlanHistory();
      } catch (err) {
        alert('创建处置计划失败: ' + (err.message || err));
      }
      btn.disabled = false;
      btn.textContent = '按序执行';
    }

    async function loadPlanHistory() {
      const area = overlay.querySelector('#plan-history');
      if (!area) return;
      let runs = [];
      try { runs = (await api.remedyPlans(a.id)).items || []; } catch {}
      if (!runs.length) { area.innerHTML = ''; return; }
      const RUN_TEXT = { pending: '待执行', running: '执行中', waiting_approval: '待审批', done: '已完成', stopped: '已停止', failed: '失败' };
      area.innerHTML = `<h4 style="margin-top:14px">处置历史</h4><ul style="list-style:none;margin:0;padding:0">${
        runs.map(r => `<li style="display:flex;gap:8px;align-items:center;padding:6px 8px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:4px;font-size:12px;background:var(--bg)">
          <code>${esc(r.id)}</code><span style="color:var(--muted)">${r.steps ? r.steps.length : 0} 步</span>
          <span style="flex:1"></span><span style="color:var(--muted)">${esc(r.created_by || '')}</span>
          <span style="font-weight:600">${RUN_TEXT[r.status] || esc(r.status)}</span></li>`).join('')}</ul>`;
    }

    const draggable = isOperator;
    overlay.innerHTML = `<div class="modal" style="max-width:760px;max-height:84vh;overflow:auto">
      <div class="modal-header"><h3>${severityBadge(a.severity)} ${esc(a.alert_type_name)}</h3>
        <button class="btn btn-ghost btn-icon" id="detail-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <p style="font-size:12px;color:var(--muted)">告警 ID: <code>${esc(a.id)}</code> · 类型: <code>${esc(a.alert_type_id)}</code> · 状态: ${statusBadge(a.status)}</p>
        <p style="font-size:12px;color:var(--muted)">节点: ${esc(a.node_name || a.node_id)} (${esc(a.node_id)}) · 首次触发: ${fmtTime(a.first_seen)} · 最近: ${fmtTime(a.last_seen)} · 解决时间: ${fmtTime(a.resolved_at)}</p>
        <div style="margin:12px 0;padding:10px 12px;background:var(--bg);border-radius:var(--radius);font-size:13px">${esc(a.message)}</div>
        ${snapshot.length ? `<h4>指标快照</h4><table class="table"><thead><tr><th>指标</th><th>值</th></tr></thead><tbody>${snapshot.map(([k, v]) => `<tr><td><code>${esc(k)}</code></td><td>${esc(v)}</td></tr>`).join('')}</tbody></table>` : ''}
        <h4 style="margin-top:16px">可用对策 ${isOperator ? '<span style="font-weight:400;color:var(--muted);font-size:11px">（勾选多选，拖动调整执行顺序）</span>' : ''}</h4>
        ${state.remedies.length ? `<ul id="rm-list" style="list-style:none;margin:0;padding:0">${state.remedies.map(r => `
          <li class="rm-row" data-rid="${esc(r.id)}" ${draggable ? 'draggable="true"' : ''} style="border:1px solid var(--border);border-radius:var(--radius);padding:10px 12px;margin-bottom:8px;background:var(--surface);cursor:${draggable ? 'grab' : 'default'}">
            <div style="display:flex;align-items:center;gap:8px">
              ${draggable ? `<input type="checkbox" class="rm-check" title="选择执行" style="accent-color:var(--accent)">` : ''}
              ${draggable ? '<span class="rm-order" style="font-size:11px;color:var(--muted);min-width:16px;text-align:center">-</span>' : ''}
              <strong style="font-size:13px;flex:1">${esc(r.name)}</strong>
              <span style="font-size:11px;color:var(--muted)">${esc(r.kind)} · 风险 ${esc(r.risk)} · ${r.source === 'user' ? '用户自定义' : r.source === 'builtin' ? '内置' : 'AI 生成'}${r.source === 'ai' && !r.reviewed ? ' · <span style="color:var(--warn)">待审核</span>' : ''}</span>
            </div>
            <pre style="margin:8px 0 0;padding:8px;background:var(--bg);border-radius:var(--radius);font-family:var(--font-mono);font-size:12px;white-space:pre-wrap;word-break:break-all;max-height:140px;overflow:auto">${esc(r.content)}</pre>
            ${r.rollback ? `<div style="margin-top:6px;font-size:11px;color:var(--muted)">回滚: <code>${esc(r.rollback)}</code></div>` : ''}
          </li>`).join('')}</ul>
        ${isOperator ? `<div style="display:flex;gap:8px;align-items:center;margin:10px 0">
          <label style="font-size:12px;display:flex;align-items:center;gap:4px"><input type="checkbox" id="rm-stop-on-error" checked style="accent-color:var(--accent)"> 失败即停</label>
          <span style="flex:1"></span>
          <button class="btn btn-secondary btn-sm" id="rm-exec"><svg width="14" height="14" aria-hidden="true"><use href="#icon-play"/></svg> 按序执行</button>
        </div>` : ''}` : '<p style="color:var(--muted);font-size:12px">暂无对策，可在告警类型设置中补充</p>'}
        <div id="plan-area"></div>
        <div id="plan-history"></div>
      </div>
    </div>`;

    document.body.appendChild(overlay);
    overlay.querySelector('#detail-close').addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });

    // 拖拽排序 + 勾选顺序刷新
    if (draggable) {
      const list = overlay.querySelector('#rm-list');
      let dragRow = null;
      list.addEventListener('dragstart', (e) => {
        const row = e.target.closest('.rm-row');
        if (!row) return;
        dragRow = row;
        row.style.opacity = '0.5';
        e.dataTransfer.effectAllowed = 'move';
      });
      list.addEventListener('dragend', (e) => {
        const row = e.target.closest('.rm-row');
        if (row) row.style.opacity = '';
        dragRow = null;
      });
      list.addEventListener('dragover', (e) => {
        e.preventDefault();
        const row = e.target.closest('.rm-row');
        if (!row || row === dragRow) return;
        const rect = row.getBoundingClientRect();
        const after = (e.clientY - rect.top) > rect.height / 2;
        if (after) row.after(dragRow); else row.before(dragRow);
      });
      const refreshOrder = () => {
        [...list.querySelectorAll('.rm-row')].forEach((row, i) => {
          row.querySelector('.rm-order').textContent = row.querySelector('.rm-check').checked ? (i + 1) : '-';
        });
      };
      list.addEventListener('change', refreshOrder);
      refreshOrder();
    }

    overlay.querySelector('#rm-exec')?.addEventListener('click', executePlan);
    loadPlanHistory();
  }

  // ---------- 监控配置（admin） ----------

  async function renderConfig() {
    const container = document.getElementById('monitor-config');
    if (!container) return;

    const [sil, typesRes, chsRes] = await Promise.all([
      api.monitorSilence().catch(() => ({ silence_until: 0 })),
      api.alertTypes().catch(() => ({ items: [] })),
      api.notifyChannels().catch(() => ({ items: [] })),
    ]);
    const types = typesRes.items || [];
    const channels = chsRes.items || [];
    const silenceUntil = sil.silence_until || 0;

    container.innerHTML = `
      <section style="border:1px solid var(--border);border-radius:var(--radius);padding:16px;margin-bottom:16px;background:var(--surface)">
        <h3 style="margin:0 0 10px;font-size:14px">全局静默</h3>
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
          <input id="cfg-silence" type="datetime-local" value="${silenceUntil ? new Date(silenceUntil * 1000 - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ''}" style="width:220px">
          <button class="btn btn-secondary btn-sm" id="cfg-silence-set">设置静默</button>
          <button class="btn btn-ghost btn-sm" id="cfg-silence-clear" ${silenceUntil ? '' : 'disabled'}>取消静默</button>
          <span style="font-size:12px;color:var(--muted)">${silenceUntil ? '静默至 ' + new Date(silenceUntil * 1000).toLocaleString('zh-CN', { hour12: false }) : '未静默'}</span>
        </div>
      </section>

      <section style="border:1px solid var(--border);border-radius:var(--radius);padding:16px;margin-bottom:16px;background:var(--surface)">
        <h3 style="margin:0 0 10px;font-size:14px">告警类型（阈值 / 开关 / 自动执行放行）</h3>
        <div style="overflow-x:auto"><table class="table"><thead><tr><th>ID</th><th>名称</th><th>级别</th><th>阈值</th><th>持续</th><th>启用</th><th>自动放行</th><th></th></tr></thead>
        <tbody>${types.map(at => `
          <tr data-at="${esc(at.id)}">
            <td><code>${esc(at.id)}</code></td>
            <td>${esc(at.name)}</td>
            <td>${esc(at.default_severity)}</td>
            <td><input class="at-val" style="width:70px" value="${at.default_params.per_core > 0 ? at.default_params.per_core : at.default_params.value}" data-kind="${at.default_params.per_core > 0 ? 'per_core' : 'value'}">${at.default_params.per_core > 0 ? '×核' : ''}</td>
            <td><input class="at-dur" style="width:50px" value="${at.default_params.duration || 0}"> 次</td>
            <td><input type="checkbox" class="at-enabled" ${at.enabled ? 'checked' : ''}></td>
            <td><input type="checkbox" class="at-auto" ${at.auto_approve ? 'checked' : ''}></td>
            <td><button class="btn btn-secondary btn-sm at-save">保存</button></td>
          </tr>`).join('')}</tbody></table></div>
      </section>

      <section style="border:1px solid var(--border);border-radius:var(--radius);padding:16px;margin-bottom:16px;background:var(--surface)">
        <h3 style="margin:0 0 10px;font-size:14px">通知渠道</h3>
        <div id="cfg-channels" style="margin-bottom:10px">
          ${channels.length ? channels.map(ch => `
            <div style="display:flex;gap:8px;align-items:center;padding:8px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:6px;background:var(--bg)">
              <span style="font-size:12px;font-weight:600">${esc(ch.name)}</span>
              <span style="font-size:11px;color:var(--muted)">${esc(ch.kind)} · 门槛 ${esc(ch.severity_min)} · ${ch.alert_types ? '类型: ' + esc(ch.alert_types) : '全部类型'}</span>
              <span style="font-size:11px;color:${ch.enabled ? 'var(--success)' : 'var(--muted)'}">${ch.enabled ? '启用' : '停用'}</span>
              <span style="flex:1"></span>
              <button class="btn btn-ghost btn-sm" data-test-ch="${esc(ch.id)}">测试</button>
              <button class="btn btn-ghost btn-sm" data-del-ch="${esc(ch.id)}">删除</button>
            </div>`).join('') : '<p style="color:var(--muted);font-size:12px">暂无渠道</p>'}
        </div>
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
          <input id="ch-name" placeholder="渠道名" style="width:110px">
          <select id="ch-kind" style="width:100px"><option value="webhook">Webhook</option><option value="email">邮件</option></select>
          <input id="ch-sev" placeholder="门槛(warn)" style="width:90px" value="warn">
          <input id="ch-types" placeholder="告警ID(空=全部,逗号分隔)" style="width:190px">
          <input id="ch-url" placeholder="Webhook URL / SMTP host:port" style="width:210px">
          <input id="ch-user" placeholder="账号(可选)" style="width:100px">
          <input id="ch-pass" type="password" placeholder="密码(可选)" style="width:100px">
          <input id="ch-from" placeholder="发件人(邮件)" style="width:110px">
          <input id="ch-to" placeholder="收件人,逗号分隔(邮件)" style="width:180px">
          <button class="btn btn-secondary btn-sm" id="ch-add">添加</button>
        </div>
      </section>
    `;

    // 静默
    document.getElementById('cfg-silence-set')?.addEventListener('click', async () => {
      const v = document.getElementById('cfg-silence').value;
      if (!v) return alert('请选择静默截止时间');
      const until = Math.floor(new Date(v).getTime() / 1000);
      try { await api.setMonitorSilence(until); renderConfig(); } catch (e) { alert('设置失败: ' + (e.message || e)); }
    });
    document.getElementById('cfg-silence-clear')?.addEventListener('click', async () => {
      try { await api.setMonitorSilence(0); renderConfig(); } catch (e) { alert('取消失败: ' + (e.message || e)); }
    });

    // 告警类型保存
    container.querySelectorAll('.at-save').forEach(btn => {
      btn.addEventListener('click', async () => {
        const tr = btn.closest('tr');
        const id = tr.dataset.at;
        const at = types.find(x => x.id === id);
        if (!at) return;
        const kind = tr.querySelector('.at-val').dataset.kind;
        if (kind === 'per_core') { at.default_params.per_core = parseFloat(tr.querySelector('.at-val').value) || 1.5; }
        else { at.default_params.value = parseFloat(tr.querySelector('.at-val').value) || 0; }
        at.default_params.duration = parseInt(tr.querySelector('.at-dur').value) || 1;
        at.enabled = tr.querySelector('.at-enabled').checked;
        at.auto_approve = tr.querySelector('.at-auto').checked;
        try { await api.updateAlertType(id, at); btn.textContent = '✓ 已保存'; setTimeout(() => { btn.textContent = '保存'; }, 1500); } catch (e) { alert('保存失败: ' + (e.message || e)); }
      });
    });

    // 通知渠道：测试/删除
    container.querySelectorAll('[data-test-ch]').forEach(btn => {
      btn.addEventListener('click', async () => {
        btn.disabled = true;
        try { await api.testNotifyChannel(btn.dataset.testCh); alert('测试发送成功'); } catch (e) { alert('测试发送失败: ' + (e.message || e)); }
        btn.disabled = false;
      });
    });
    container.querySelectorAll('[data-del-ch]').forEach(btn => {
      btn.addEventListener('click', async () => {
        if (!confirm('删除通知渠道?')) return;
        try { await api.deleteNotifyChannel(btn.dataset.delCh); renderConfig(); } catch (e) { alert('删除失败: ' + (e.message || e)); }
      });
    });

    // 通知渠道：添加
    document.getElementById('ch-add')?.addEventListener('click', async () => {
      const name = document.getElementById('ch-name').value.trim();
      const kind = document.getElementById('ch-kind').value;
      if (!name) return alert('请填写渠道名');
      const ch = {
        id: 'CH-' + Date.now(), kind, name,
        severity_min: document.getElementById('ch-sev').value.trim() || 'warn',
        alert_types: document.getElementById('ch-types').value.trim(),
        enabled: true,
      };
      if (kind === 'webhook') {
        ch.config = { webhook: { url: document.getElementById('ch-url').value.trim(), headers: {} } };
      } else {
        const hp = document.getElementById('ch-url').value.trim().split(':');
        ch.config = { email: {
          smtp_host: hp[0] || '', smtp_port: parseInt(hp[1]) || 465,
          username: document.getElementById('ch-user').value.trim(),
          password: document.getElementById('ch-pass').value,
          from: document.getElementById('ch-from').value.trim(),
          to: document.getElementById('ch-to').value.split(',').map(x => x.trim()).filter(Boolean),
        } };
      }
      try { await api.createNotifyChannel(ch); renderConfig(); } catch (e) { alert('添加失败: ' + (e.message || e)); }
    });
  }

  renderView();
}
