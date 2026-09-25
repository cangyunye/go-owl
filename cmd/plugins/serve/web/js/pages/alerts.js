// 告警页：告警列表（分组面板/状态级别/类型/节点筛选、分页）、处置（确认/解决）、
// 详情与对策（查看/复制，编辑/新增/删除仅 admin）、监控配置（静默/告警类型/通知渠道，admin）。
export function renderAlerts(render, navigate, user, api, shell, scope) {
  const isOperator = ['operator', 'admin'].includes(user.role);
  const isAdmin = user.role === 'admin';
  const pageSize = 20;
  const state = scope.restoreState({
    status: 'active', severity: '', typeId: '', nodeId: '', page: 1, total: 0, items: [],
    detail: null, remedies: [], types: [], allGroups: [], selectedGroups: [], groupSearch: '',
  });
  let mode = scope.restoreState({ mode: 'list' }).mode; // list | config

  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - (t * 1000)) / 1000); if (s < 60) return s + '秒前'; if (s < 3600) return Math.floor(s / 60) + '分钟前'; if (s < 86400) return Math.floor(s / 3600) + '小时前'; return Math.floor(s / 86400) + '天前'; }
  function fmtTime(t) { return t ? new Date(t * 1000).toLocaleString('zh-CN', { hour12: false }) : '-'; }
  function tagColor(s) { let h = 0; for (let i = 0; i < s.length; i++) h = ((h << 5) - h) + s.charCodeAt(i); return 'tag-r' + (Math.abs(h) % 12); }

  const STATUS_TEXT = { open: '待处理', acked: '已确认', resolved: '已解决' };
  scope.persistState(() => ({ status: state.status, severity: state.severity, typeId: state.typeId, nodeId: state.nodeId, page: state.page, selectedGroups: state.selectedGroups, groupSearch: state.groupSearch, mode }));
  const STATUS_CLS = { open: 'pending', acked: 'info', resolved: 'success' };
  const SEV_TEXT = { critical: '紧急', warn: '警告', info: '提示' };
  const SEV_COLOR = { critical: 'var(--danger)', warn: 'var(--warn)', info: 'var(--info)' };
  const REMEDY_KIND_TEXT = { script: '自定义命令', playbook: '剧本', sop: '人工指引' };

  function severityBadge(s) {
    const color = SEV_COLOR[s] || 'var(--muted)';
    return `<span style="display:inline-block;padding:2px 8px;border-radius:999px;font-size:var(--fs-xs);font-weight:600;background:color-mix(in oklch, ${color} 15%, var(--surface));color:${color}">${SEV_TEXT[s] || esc(s)}</span>`;
  }

  function statusBadge(s) {
    return `<span class="hi-status ${STATUS_CLS[s] || 'pending'}">${STATUS_TEXT[s] || esc(s)}</span>`;
  }

  function buildParams() {
    const p = { status: state.status, severity: state.severity, limit: pageSize, offset: (state.page - 1) * pageSize };
    if (state.nodeId) p.node_id = state.nodeId;
    if (state.typeId) p.alert_type_id = state.typeId;
    if (state.selectedGroups.length) p.group = state.selectedGroups.join(',');
    return p;
  }

  // ---------- 左侧面板：分组筛选（多选，仿节点管理） ----------

  function renderPanel() {
    scope.panel.setTitle('告警分组');
    const q = state.groupSearch.toLowerCase();
    const filtered = state.allGroups.filter(g => !q || g.toLowerCase().includes(q));
    scope.panel.setContent(`
      <div style="padding:6px 10px">
        <input type="text" id="alert-group-search" placeholder="搜索分组…" style="width:100%;padding:6px 8px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface);color:var(--fg);font-size:var(--fs-xs);outline:none" value="${esc(state.groupSearch)}">
      </div>
      <div class="group-chips">
        ${filtered.map(g => {
          const active = state.selectedGroups.includes(g);
          return `<button type="button" class="group-chip ${tagColor(g)} ${active ? 'selected' : ''}" data-group="${esc(g)}" aria-pressed="${active}">
            <span class="dot"></span><span class="group-text">${esc(g)}</span>
          </button>`;
        }).join('') || '<div style="padding:10px 12px;font-size:var(--fs-xs);color:var(--muted)">暂无分组</div>'}
      </div>
      <div style="padding:10px 12px;font-size:var(--fs-xs);color:var(--muted)">点击分组（可多选）过滤右侧告警历史</div>
    `);
    document.getElementById('alert-group-search')?.addEventListener('input', (e) => {
      state.groupSearch = e.target.value;
      renderPanel();
    });
    document.querySelectorAll('#panelList .group-chip').forEach(chip => {
      chip.addEventListener('click', () => {
        const g = chip.dataset.group;
        const i = state.selectedGroups.indexOf(g);
        if (i >= 0) state.selectedGroups.splice(i, 1); else state.selectedGroups.push(g);
        state.page = 1;
        renderPanel();
        load();
      });
    });
  }

  async function loadPanel() {
    let groups = [];
    try {
      const res = await api.filters();
      groups = (res && res.groups) || [];
    } catch { /* 分组拉取失败不阻塞页面 */ }
    state.allGroups = [...new Set(groups)].sort();
    renderPanel();
  }

  // ---------- 视图骨架 ----------

  function renderView() {
    render(`
      <div class="view-head">
        <h2 class="view-title">告警中心</h2>
        <div class="view-actions" id="alert-actions" style="display:flex;gap:8px;align-items:center">
          ${isAdmin ? `<button class="btn btn-ghost btn-sm ${mode === 'config' ? 'active' : ''}" id="toggle-config"><svg width="14" height="14" aria-hidden="true"><use href="#icon-settings"/></svg> 监控配置</button>` : ''}
          <button class="btn btn-secondary btn-sm" id="alert-refresh"><svg width="14" height="14" aria-hidden="true"><use href="#icon-refresh"/></svg> 刷新</button>
          <span id="alert-count" style="font-size:var(--fs-xs);color:var(--muted)"></span>
        </div>
      </div>
      ${mode === 'list' ? `
      <div class="alert-filter-bar" id="alert-filter-bar"></div>
      <div id="alert-list"></div>
      <div class="pagination" id="alert-pagination"></div>
      ` : `<div id="monitor-config"></div>`}
    `, afterRender);
    document.getElementById('toggle-config')?.addEventListener('click', () => {
      mode = mode === 'list' ? 'config' : 'list';
      renderView();
    });
    if (mode === 'list') {
      renderFilterBar();
      load();
    } else {
      renderConfig();
    }
  }

  function afterRender() { return null; }

  // ---------- 上方筛选栏：状态 / 级别 / 告警类型 / 节点 ----------

  function renderFilterBar() {
    const bar = document.getElementById('alert-filter-bar');
    if (!bar) return;
    const statusOpts = [
      { key: 'active', label: '活跃' }, { key: 'open', label: '待处理' },
      { key: 'acked', label: '已确认' }, { key: 'resolved', label: '已解决' }, { key: '', label: '全部' },
    ];
    const sevOpts = [
      { key: '', label: '全部级别' }, { key: 'critical', label: '紧急' }, { key: 'warn', label: '警告' }, { key: 'info', label: '提示' },
    ];
    bar.innerHTML = `
      <span class="afb-label">状态</span>
      <span class="afb-group">${statusOpts.map(f => `<button type="button" class="filter-chip ${state.status === f.key ? 'selected' : ''}" data-status="${f.key}">${f.label}</button>`).join('')}</span>
      <span class="afb-sep"></span>
      <span class="afb-label">级别</span>
      <span class="afb-group">${sevOpts.map(f => `<button type="button" class="filter-chip ${state.severity === f.key ? 'selected' : ''}" data-sev="${f.key}"><span class="dot" style="background:${SEV_COLOR[f.key] || 'var(--muted)'}"></span>${f.label}</button>`).join('')}</span>
      <span class="afb-sep"></span>
      <select id="alert-type" aria-label="按告警类型筛选">
        <option value="">全部类型</option>
        ${state.types.map(t => `<option value="${esc(t.id)}" ${state.typeId === t.id ? 'selected' : ''}>${esc(t.name)}</option>`).join('')}
      </select>
      <input type="text" id="alert-node" placeholder="按节点 ID 过滤" style="width:150px" value="${esc(state.nodeId || '')}">
      <button class="btn btn-secondary btn-sm" id="alert-search">查询</button>
    `;
    bar.querySelectorAll('[data-status]').forEach(el => {
      el.addEventListener('click', () => {
        state.status = el.dataset.status; state.page = 1;
        bar.querySelectorAll('[data-status]').forEach(x => x.classList.toggle('selected', x === el));
        load();
      });
    });
    bar.querySelectorAll('[data-sev]').forEach(el => {
      el.addEventListener('click', () => {
        state.severity = el.dataset.sev; state.page = 1;
        bar.querySelectorAll('[data-sev]').forEach(x => x.classList.toggle('selected', x === el));
        load();
      });
    });
    document.getElementById('alert-type')?.addEventListener('change', (e) => {
      state.typeId = e.target.value; state.page = 1; load();
    });
    document.getElementById('alert-search')?.addEventListener('click', () => {
      state.nodeId = document.getElementById('alert-node').value.trim(); state.page = 1; load();
    });
    document.getElementById('alert-node')?.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') document.getElementById('alert-search').click();
    });
  }

  // ---------- 列表与分页 ----------

  function renderList() {
    const list = document.getElementById('alert-list');
    if (!list) return;
    if (!state.items.length) {
      list.innerHTML = '<div class="view-empty" style="padding:40px"><div class="empty-title">暂无告警</div><div style="font-size:var(--fs-xs);color:var(--muted);margin-top:6px">监控采集每 60 秒执行一次，阈值规则见告警类型设置</div></div>';
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
    try {
      const res = await api.alerts(buildParams());
      state.items = res.items || [];
      state.total = res.total || 0;
    } catch { state.items = []; state.total = 0; }
    renderList();
  }

  // ---------- 告警详情与对策 ----------

  function remedyContentText(r) {
    if (r.kind === 'playbook') return `剧本: ${r.content}`;
    return r.content || '';
  }

  async function copyText(text, btn) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const ta = document.createElement('textarea');
      ta.value = text; scope.resources.overlay(ta); ta.select();
      try { document.execCommand('copy'); } catch {}
      ta.remove();
    }
    if (btn) { const t = btn.textContent; btn.textContent = '✓ 已复制'; scope.resources.setTimeout(() => { btn.textContent = t; }, 1200); }
  }

  async function openDetail(id) {
    let rec;
    try { rec = await api.alert(id); } catch (err) { alert('加载详情失败: ' + (err.message || err)); return; }
    const a = rec.alert || {};
    state.detail = a;
    state.remedies = rec.remedies || [];
    const bindings = (await api.alertBindings(id).catch(() => ({ items: [] }))).items || [];
    state.types = state.types.length ? state.types : (await api.alertTypes().catch(() => ({ items: [] }))).items || [];

    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay open';
    const snapshot = (() => { try { return Object.entries(JSON.parse(a.metric_snapshot || '{}')); } catch { return []; } })();

    // 闭环验证徽标：recovered=疗效确认 / not_recovered=未消除 / inconclusive=无法判定
    const VERIFY_TEXT = { recovered: '✅ 疗效确认', not_recovered: '⚠️ 告警未消除', inconclusive: '❔ 无法判定' };
    const VERIFY_COLOR = { recovered: 'var(--success)', not_recovered: 'var(--warn)', inconclusive: 'var(--muted)' };
    function verifyBadgeHtml(v) {
      if (!v || !v.status) return '';
      return `<span title="${esc(v.message || '')}" style="font-size:var(--fs-xs);font-weight:600;color:${VERIFY_COLOR[v.status] || 'var(--muted)'}">${VERIFY_TEXT[v.status] || esc(v.status)}</span>`;
    }

    function planProgressHtml(plan, verification) {
      const RUN_TEXT = { pending: '待执行', running: '执行中', waiting_approval: '待审批', done: '已完成', stopped: '已停止', failed: '失败' };
      const STEP_TEXT = { pending: '等待', running: '执行中', success: '成功', failed: '失败', skipped: '跳过', pending_approval: '待审批' };
      const STEP_COLOR = { pending: 'var(--muted)', running: 'var(--info)', success: 'var(--success)', failed: 'var(--danger)', skipped: 'var(--muted)', pending_approval: 'var(--warn)' };
      const steps = (plan.steps || []).map(st => `
        <li style="display:flex;gap:8px;align-items:flex-start;padding:8px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:6px;background:var(--bg)">
          <span style="min-width:18px;text-align:center;font-weight:600;color:var(--muted)">${st.order + 1}</span>
          <div style="flex:1;min-width:0">
            <div style="font-size:var(--fs-xs);font-weight:600">${esc(st.name)} <span style="color:var(--muted);font-weight:400">· ${esc(st.kind)}</span></div>
            ${st.output ? `<pre style="margin:4px 0 0;padding:6px;background:var(--surface);border-radius:var(--radius);font-family:var(--font-mono);font-size:var(--fs-xs);white-space:pre-wrap;word-break:break-all;max-height:120px;overflow:auto">${esc(st.output)}</pre>` : ''}
          </div>
          <span style="font-size:var(--fs-xs);font-weight:600;color:${STEP_COLOR[st.status] || 'var(--muted)'}">${STEP_TEXT[st.status] || esc(st.status)}</span>
        </li>`).join('');
      return `<div style="border:1px solid var(--border);border-radius:var(--radius);padding:12px;margin:12px 0;background:var(--surface)">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">
          <strong style="font-size:var(--fs-sm)">处置计划 <code>${esc(plan.id)}</code>${verifyBadgeHtml(verification)}</strong>
          <span style="display:flex;gap:6px;align-items:center">
            <span style="font-size:var(--fs-xs);font-weight:600;color:${plan.status === 'done' ? 'var(--success)' : plan.status === 'failed' || plan.status === 'stopped' ? 'var(--danger)' : plan.status === 'waiting_approval' ? 'var(--warn)' : 'var(--info)'}">${RUN_TEXT[plan.status] || esc(plan.status)}</span>
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
        // 终态后继续轮询一小段，等待闭环验证结果落库
        if (plan.status === 'done' && !res.verification && (loadPlanProgress._tries?.[planId] || 0) < 40) {
          loadPlanProgress._tries = loadPlanProgress._tries || {};
          loadPlanProgress._tries[planId] = (loadPlanProgress._tries[planId] || 0) + 1;
          scope.resources.setTimeout(() => loadPlanProgress(planId, area), 1500);
          area.innerHTML = planProgressHtml(plan, null);
          return;
        }
        loadPlanProgress._tries = loadPlanProgress._tries || {};
        delete loadPlanProgress._tries[planId];
        area.innerHTML = planProgressHtml(plan, res.verification);
        return;
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
        scope.resources.setTimeout(() => loadPlanProgress(planId, area), 1500);
      }
    }

    async function executePlan() {
      const rows = [...overlay.querySelectorAll('#rm-list .rm-row')];
      const selected = rows.filter(r => r.querySelector('.rm-check')?.checked);
      if (!selected.length) { alert('请先勾选要执行的对策'); return; }
      const remedyIds = selected.map(r => r.dataset.rid);
      const stopOnError = overlay.querySelector('#rm-stop-on-error')?.checked !== false;
      const btn = overlay.querySelector('#rm-exec');
      btn.disabled = true;
      btn.textContent = '执行中...';
      try {
        const res = await api.createRemedyPlan(a.id, { remedy_ids: remedyIds, stop_on_error: stopOnError });
        const planArea = overlay.querySelector('#plan-area');
        planArea.innerHTML = '<p style="color:var(--muted);font-size:var(--fs-xs)">计划已创建，执行中...</p>';
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
      let items = [];
      try { items = (await api.remedyPlans(a.id)).items || []; } catch {}
      if (!items.length) { area.innerHTML = ''; return; }
      const RUN_TEXT = { pending: '待执行', running: '执行中', waiting_approval: '待审批', done: '已完成', stopped: '已停止', failed: '失败' };
      area.innerHTML = `<h4 style="margin-top:14px">处置历史</h4><ul style="list-style:none;margin:0;padding:0">${
        items.map(it => {
          const r = it.run || it;
          return `<li style="display:flex;gap:8px;align-items:center;padding:6px 8px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:4px;font-size:var(--fs-xs);background:var(--bg)">
          <code>${esc(r.id)}</code><span style="color:var(--muted)">${r.steps ? r.steps.length : 0} 步</span>
          <span style="flex:1"></span><span style="color:var(--muted)">${esc(r.created_by || '')}</span>
          ${verifyBadgeHtml(it.verification)}
          <span style="font-weight:600">${RUN_TEXT[r.status] || esc(r.status)}</span></li>`;
        }).join('')}</ul>`;
    }

    const draggable = isOperator;
    overlay.innerHTML = `<div class="modal" style="max-width:760px;max-height:84vh;overflow:auto">
      <div class="modal-header"><h3>${severityBadge(a.severity)} ${esc(a.alert_type_name)}</h3>
        <button class="btn btn-ghost btn-icon" id="detail-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <p style="font-size:var(--fs-xs);color:var(--muted)">告警 ID: <code>${esc(a.id)}</code> · 类型: <code>${esc(a.alert_type_id)}</code> · 状态: ${statusBadge(a.status)}</p>
        <p style="font-size:var(--fs-xs);color:var(--muted)">节点: ${esc(a.node_name || a.node_id)} (${esc(a.node_id)}) · 首次触发: ${fmtTime(a.first_seen)} · 最近: ${fmtTime(a.last_seen)} · 解决时间: ${fmtTime(a.resolved_at)}</p>
        <div style="margin:12px 0;padding:10px 12px;background:var(--bg);border-radius:var(--radius);font-size:var(--fs-sm)">${esc(a.message)}</div>
        ${snapshot.length ? `<h4>指标快照</h4><table class="table"><thead><tr><th>指标</th><th>值</th></tr></thead><tbody>${snapshot.map(([k, v]) => `<tr><td><code>${esc(k)}</code></td><td>${esc(v)}</td></tr>`).join('')}</tbody></table>` : ''}
        <h4 style="margin-top:16px">可用对策 ${isOperator ? '<span style="font-weight:400;color:var(--muted);font-size:var(--fs-xs)">（勾选多选，拖动调整执行顺序）</span>' : ''}
          ${isAdmin ? `<button class="btn btn-secondary btn-sm" id="rm-add" style="margin-left:8px">+ 添加指令</button>` : ''}
        </h4>
        ${state.remedies.length ? `<ul id="rm-list" style="list-style:none;margin:0;padding:0">${state.remedies.map(r => `
          <li class="remedy-row rm-row" data-rid="${esc(r.id)}" ${draggable ? 'draggable="true"' : ''} style="cursor:${draggable ? 'grab' : 'default'}">
            <div class="rm-head">
              ${draggable ? `<input type="checkbox" class="rm-check" title="选择执行" style="accent-color:var(--accent)">` : ''}
              ${draggable ? '<span class="rm-order" style="font-size:var(--fs-xs);color:var(--muted);min-width:16px;text-align:center">-</span>' : ''}
              <strong style="font-size:var(--fs-sm);flex:1">${esc(r.name)}</strong>
              <span style="font-size:var(--fs-xs);color:var(--muted)">${REMEDY_KIND_TEXT[r.kind] || esc(r.kind)} · 风险 ${esc(r.risk)} · ${r.source === 'user' ? '用户自定义' : r.source === 'builtin' ? '内置' : 'AI 生成'}${r.source === 'ai' && !r.reviewed ? ' · <span style="color:var(--warn)">待审核</span>' : ''}</span>
              <button class="btn btn-ghost btn-sm" data-copy-rm="${esc(r.id)}" title="复制内容">复制</button>
              ${isAdmin ? `<button class="btn btn-ghost btn-sm" data-edit-rm="${esc(r.id)}">编辑</button>` : ''}
            </div>
            <pre class="rm-content">${esc(remedyContentText(r))}</pre>
            ${r.rollback ? `<div style="margin-top:6px;font-size:var(--fs-xs);color:var(--muted)">回滚: <code>${esc(r.rollback)}</code></div>` : ''}
          </li>`).join('')}</ul>
        ${isOperator ? `<div style="display:flex;gap:8px;align-items:center;margin:10px 0">
          <label style="font-size:var(--fs-xs);display:flex;align-items:center;gap:4px"><input type="checkbox" id="rm-stop-on-error" checked style="accent-color:var(--accent)"> 失败即停</label>
          <span style="flex:1"></span>
          <button class="btn btn-secondary btn-sm" id="rm-exec"><svg width="14" height="14" aria-hidden="true"><use href="#icon-play"/></svg> 按序执行</button>
        </div>` : ''}` : `<p style="color:var(--muted);font-size:var(--fs-xs)">暂无对策${isAdmin ? '，可点击上方"添加指令"补充' : '，可联系管理员在告警类型下补充'}</p>`}
        <h4 style="margin-top:16px">本告警专属指令 <span style="font-weight:400;color:var(--muted);font-size:var(--fs-xs)">（按告警 ID 绑定，重开保留）</span>
          ${isAdmin ? '<button class="btn btn-secondary btn-sm" id="ab-add" style="margin-left:8px">+ 添加指令</button>' : ''}
        </h4>
        ${bindings.length ? `<ul id="ab-list" style="list-style:none;margin:0;padding:0">${bindings.map((b, i) => `
          <li class="remedy-row ab-row" data-abid="${esc(b.id)}">
            <div class="rm-head">
              ${isOperator ? '<input type="checkbox" class="ab-check" title="选择执行" style="accent-color:var(--accent)">' : ''}
              <span class="rm-order">${i + 1}</span>
              <strong style="font-size:var(--fs-sm);flex:1">${esc(b.name)}</strong>
              <span style="font-size:var(--fs-xs);color:var(--muted)">${b.kind === 'playbook' ? '剧本' : '命令'} · ${b.auto_exec ? '<span style="color:var(--warn)">自动</span>' : '手动'} · ${b.exec_mode === 'concurrent' ? '并发' : '串行'}</span>
              ${isOperator ? `<button class="btn btn-secondary btn-sm" data-ab-run="${esc(b.id)}">执行</button>` : ''}
              ${isAdmin ? `
                <button class="btn btn-ghost btn-sm" data-ab-up="${i}" ${i === 0 ? 'disabled' : ''}>↑</button>
                <button class="btn btn-ghost btn-sm" data-ab-down="${i}" ${i === bindings.length - 1 ? 'disabled' : ''}>↓</button>
                <button class="btn btn-ghost btn-sm" data-ab-edit="${esc(b.id)}">编辑</button>
                <button class="btn btn-ghost btn-sm" data-ab-del="${esc(b.id)}" style="color:var(--danger)">删除</button>` : ''}
            </div>
            <pre class="rm-content">${esc(b.kind === 'playbook' ? '剧本: ' + b.content : b.content)}</pre>
          </li>`).join('')}</ul>
        ${isOperator ? `<div style="display:flex;gap:8px;align-items:center;margin:10px 0">
          <span style="font-size:var(--fs-xs);color:var(--muted)">执行方式</span>
          <select id="ab-mode" style="width:auto">
            <option value="sequential">顺序串行</option>
            <option value="concurrent">并发执行</option>
          </select>
          <span style="flex:1"></span>
          <button class="btn btn-secondary btn-sm" id="ab-run"><svg width="14" height="14" aria-hidden="true"><use href="#icon-play"/></svg> 执行勾选指令</button>
        </div>` : ''}` : `<p style="color:var(--muted);font-size:var(--fs-xs)">暂无专属指令${isAdmin ? '，可点击上方「添加指令」绑定剧本或脚本' : ''}</p>`}
        <div id="binding-runs-area"></div>
        <div id="plan-area"></div>
        <div id="plan-history"></div>
      </div>
    </div>`;

    scope.resources.overlay(overlay);
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
          row.querySelector('.rm-order').textContent = row.querySelector('.rm-check')?.checked ? (i + 1) : '-';
        });
      };
      list.addEventListener('change', refreshOrder);
      refreshOrder();
    }

    // 复制 / 编辑 / 添加
    overlay.querySelectorAll('[data-copy-rm]').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const r = state.remedies.find(x => x.id === btn.dataset.copyRm);
        if (r) await copyText(remedyContentText(r), btn);
      });
    });
    const refreshDetail = () => { overlay.remove(); openDetail(a.id); };
    overlay.querySelector('#rm-add')?.addEventListener('click', () =>
      openRemedyEditor({ alertTypeId: a.alert_type_id, existing: null, onSaved: refreshDetail }));
    overlay.querySelectorAll('[data-edit-rm]').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const r = state.remedies.find(x => x.id === btn.dataset.editRm);
        if (r) openRemedyEditor({ alertTypeId: a.alert_type_id, existing: r, onSaved: refreshDetail });
      });
    });

    overlay.querySelector('#rm-exec')?.addEventListener('click', executePlan);
    loadPlanHistory();

    // ---- 专属指令：添加/编辑/删除/排序/执行/历史 ----
    overlay.querySelector('#ab-add')?.addEventListener('click', () => openBindingEditor(overlay, a, null));
    overlay.querySelectorAll('[data-ab-edit]').forEach(btn => btn.addEventListener('click', (e) => {
      e.stopPropagation();
      const b = bindings.find(x => x.id === btn.dataset.abEdit);
      if (b) openBindingEditor(overlay, a, b);
    }));
    overlay.querySelectorAll('[data-ab-del]').forEach(btn => btn.addEventListener('click', async (e) => {
      e.stopPropagation();
      if (!confirm('删除该专属指令?')) return;
      try { await api.deleteAlertBinding(btn.dataset.abDel); refreshDetail(); }
      catch (err) { alert('删除失败: ' + (err.message || err)); }
    }));
    overlay.querySelectorAll('[data-ab-up],[data-ab-down]').forEach(btn => btn.addEventListener('click', async (e) => {
      e.stopPropagation();
      const isUp = btn.dataset.abUp !== undefined;
      const i = parseInt(isUp ? btn.dataset.abUp : btn.dataset.abDown, 10);
      const j = isUp ? i - 1 : i + 1;
      if (j < 0 || j >= bindings.length) return;
      try {
        await api.updateAlertBinding(bindings[i].id, { seq: bindings[j].seq });
        await api.updateAlertBinding(bindings[j].id, { seq: bindings[i].seq });
        refreshDetail();
      } catch (err) { alert('排序失败: ' + (err.message || err)); }
    }));
    overlay.querySelectorAll('[data-ab-run]').forEach(btn => btn.addEventListener('click', async (e) => {
      e.stopPropagation();
      btn.disabled = true;
      try {
        await api.runAlertBindings(a.id, { binding_ids: [btn.dataset.abRun], mode: 'sequential' });
        loadBindingRuns();
      } catch (err) { alert('执行失败: ' + (err.message || err)); }
      btn.disabled = false;
    }));
    overlay.querySelector('#ab-run')?.addEventListener('click', async () => {
      const ids = [...overlay.querySelectorAll('.ab-row')]
        .filter(r => r.querySelector('.ab-check')?.checked)
        .map(r => r.dataset.abid);
      if (!ids.length) return alert('请先勾选要执行的指令');
      const mode = overlay.querySelector('#ab-mode')?.value || 'sequential';
      try {
        await api.runAlertBindings(a.id, { binding_ids: ids, mode });
        loadBindingRuns();
      } catch (err) { alert('执行失败: ' + (err.message || err)); }
    });

    async function loadBindingRuns() {
      if (!overlay.isConnected) return;
      const area = overlay.querySelector('#binding-runs-area');
      if (!area) return;
      let runs = [];
      try { runs = (await api.alertBindingRuns(a.id)).items || []; } catch {}
      const ST = { pending: '排队中', running: '执行中', success: '成功', failed: '失败' };
      const SC = { pending: 'var(--muted)', running: 'var(--info)', success: 'var(--success)', failed: 'var(--danger)' };
      area.innerHTML = runs.length ? `<h4 style="margin-top:14px">指令执行记录</h4><ul style="list-style:none;margin:0;padding:0">${
        runs.map(r => `<li style="display:flex;gap:8px;align-items:center;padding:6px 8px;border:1px solid var(--border);border-radius:var(--radius);margin-bottom:4px;font-size:var(--fs-xs);background:var(--bg)">
          <span style="font-weight:600;color:${SC[r.status] || 'var(--muted)'}">${ST[r.status] || esc(r.status)}</span>
          <span style="color:var(--muted)">${r.kind === 'playbook' ? '剧本' : '命令'}</span>
          <code style="font-size:var(--fs-xs)">${esc(r.ref_id || '')}</code>
          ${r.err ? `<span style="color:var(--danger);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(r.err)}">${esc(r.err)}</span>` : '<span style="flex:1"></span>'}
          <span style="color:var(--muted)">${esc(r.created_by || '')}</span>
        </li>`).join('')}</ul>` : '';
      if (runs.some(r => r.status === 'pending' || r.status === 'running')) {
        scope.resources.setTimeout(loadBindingRuns, 2000);
      }
    }
    loadBindingRuns();
  }

  // ---------- 对策编辑对话框（admin）：自定义命令 / 加载剧本 / 人工指引 ----------
  // opts: { alertTypeId, existing, onSaved }；onSaved 在保存/删除成功后回调（由调用方刷新视图）。

  async function openRemedyEditor(opts) {
    const existing = opts.existing || null;
    const isNew = !existing;
    const r = existing ? { ...existing } : {
      id: 'RM-' + Date.now(), alert_type_id: opts.alertTypeId, name: '',
      kind: 'script', content: '', risk: 'medium', rollback: '', source: 'user',
    };
    let playbooks = [];
    try { playbooks = (await api.playbooks()).data || []; } catch { /* 剧本库不可用时不阻塞 */ }

    const editor = document.createElement('div');
    editor.className = 'modal-overlay open';
    const kindNote = {
      script: '在告警节点上以 shell 执行的命令（通过 SSH，谨慎编写）',
      playbook: '从剧本库选择一个剧本，内容保存为剧本名；执行时请在剧本管理中运行',
      sop: '人工处置步骤说明，不会自动执行',
    };
    editor.innerHTML = `<div class="modal" style="max-width:640px;max-height:84vh;overflow:auto">
      <div class="modal-header"><h3>${isNew ? '添加处置指令' : '编辑处置指令'}</h3>
        <button class="btn btn-ghost btn-icon" id="re-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <div class="param-group">
          <div class="param-row"><label>名称</label></div>
          <input id="re-name" type="text" value="${esc(r.name)}" placeholder="如：清理大日志文件" style="width:100%">
          <div class="param-row"><label>类型</label></div>
          <select id="re-kind" style="width:100%">
            <option value="script" ${r.kind === 'script' ? 'selected' : ''}>自定义命令（script）</option>
            <option value="playbook" ${r.kind === 'playbook' ? 'selected' : ''}>加载剧本（playbook）</option>
            <option value="sop" ${r.kind === 'sop' ? 'selected' : ''}>人工指引（sop）</option>
          </select>
          <div class="cfg-hint" id="re-kind-note">${kindNote[r.kind] || ''}</div>
          <div id="re-playbook-wrap" style="display:${r.kind === 'playbook' ? 'block' : 'none'}">
            <div class="param-row"><label>从剧本库选择</label></div>
            <select id="re-playbook" style="width:100%">
              <option value="">— 选择剧本 —</option>
              ${playbooks.map(pb => `<option value="${esc(pb.name)}" ${r.content === pb.name ? 'selected' : ''}>${esc(pb.name)}</option>`).join('')}
            </select>
            ${playbooks.length ? '' : '<div class="cfg-hint" style="margin-top:4px">剧本库为空或不可用，请先在「剧本管理」上传剧本</div>'}
          </div>
          <div class="param-row"><label>内容</label></div>
          <textarea id="re-content" rows="8" placeholder="${r.kind === 'sop' ? '1. 第一步…\n2. 第二步…' : 'echo 命令'}" style="width:100%;font-family:var(--font-mono);font-size:var(--fs-xs);${r.kind === 'playbook' ? 'display:none' : ''}">${esc(r.kind === 'playbook' ? '' : (r.content || ''))}</textarea>
          <div class="param-row"><label>风险等级</label></div>
          <select id="re-risk" style="width:140px">
            <option value="low" ${r.risk === 'low' ? 'selected' : ''}>low</option>
            <option value="medium" ${r.risk === 'medium' ? 'selected' : ''}>medium</option>
            <option value="high" ${r.risk === 'high' ? 'selected' : ''}>high</option>
          </select>
          <div class="param-row"><label>回滚方式（可选）</label></div>
          <input id="re-rollback" type="text" value="${esc(r.rollback || '')}" placeholder="恢复原状所需命令" style="width:100%">
          <label style="font-size:var(--fs-xs);display:flex;align-items:center;gap:6px"><input type="checkbox" id="re-auto" ${r.auto_approve ? 'checked' : ''} style="accent-color:var(--accent)"> 允许自动执行（配合告警类型「自动放行」触发自愈）</label>
          ${existing && existing.source === 'builtin' ? '<div class="cfg-hint" style="color:var(--warn)">该对策为内置内容，修改会覆盖默认建议</div>' : ''}
        </div>
        <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:14px">
          ${isNew ? '' : '<button class="btn btn-ghost btn-sm" id="re-delete" style="margin-right:auto;color:var(--danger)">删除</button>'}
          <button class="btn btn-ghost btn-sm" id="re-cancel">取消</button>
          <button class="btn btn-secondary btn-sm" id="re-save">保存</button>
        </div>
      </div>
    </div>`;
    scope.resources.overlay(editor);

    const kindSel = editor.querySelector('#re-kind');
    kindSel.addEventListener('change', () => {
      editor.querySelector('#re-kind-note').textContent = kindNote[kindSel.value] || '';
      editor.querySelector('#re-playbook-wrap').style.display = kindSel.value === 'playbook' ? 'block' : 'none';
      editor.querySelector('#re-content').style.display = kindSel.value === 'playbook' ? 'none' : 'block';
    });

    const close = () => editor.remove();
    editor.querySelector('#re-close').addEventListener('click', close);
    editor.querySelector('#re-cancel').addEventListener('click', close);
    editor.addEventListener('click', (e) => { if (e.target === editor) close(); });

    editor.querySelector('#re-save').addEventListener('click', async () => {
      const payload = {
        ...r,
        name: editor.querySelector('#re-name').value.trim(),
        kind: kindSel.value,
        risk: editor.querySelector('#re-risk').value,
        rollback: editor.querySelector('#re-rollback').value.trim(),
        auto_approve: editor.querySelector('#re-auto').checked,
        source: existing ? existing.source : 'user',
        reviewed: existing ? existing.reviewed : true,
      };
      if (!payload.name) return alert('请填写名称');
      if (payload.kind === 'playbook') {
        payload.content = editor.querySelector('#re-playbook').value;
        if (!payload.content) return alert('请从剧本库选择剧本');
      } else {
        payload.content = editor.querySelector('#re-content').value;
        if (!payload.content.trim()) return alert('请填写内容');
      }
      try {
        if (isNew) await api.createRemedy(payload);
        else await api.updateRemedy(payload.id, payload);
        close();
        if (opts.onSaved) opts.onSaved();
      } catch (err) { alert('保存失败: ' + (err.message || err)); }
    });

    editor.querySelector('#re-delete')?.addEventListener('click', async () => {
      if (!confirm('删除该处置指令?')) return;
      try {
        await api.deleteRemedy(r.id);
        close();
        if (opts.onSaved) opts.onSaved();
      } catch (err) { alert('删除失败: ' + (err.message || err)); }
    });
  }

  // ---------- 专属指令绑定编辑器（admin）：剧本搜索多选 / 自定义命令 ----------

  async function nextBindingSeq(alertId) {
    try {
      const items = (await api.alertBindings(alertId)).items || [];
      return items.reduce((m, x) => Math.max(m, x.seq || 0), 0) + 1;
    } catch { return 1; }
  }

  async function openBindingEditor(parentOverlay, alert, existing) {
    const isNew = !existing;
    let playbooks = [];
    try { playbooks = ((await api.playbooks()).data || []).filter(p => p.file_exists !== false); } catch { /* 剧本库不可用不阻塞 */ }
    let selected = existing && existing.kind === 'playbook' ? [existing.content] : [];

    const editor = document.createElement('div');
    editor.className = 'modal-overlay open';
    editor.innerHTML = `<div class="modal" style="max-width:640px;max-height:84vh;overflow:auto">
      <div class="modal-header"><h3>${isNew ? '为本告警添加指令' : '编辑专属指令'}</h3>
        <button class="btn btn-ghost btn-icon" id="be-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <div class="param-group">
          <div class="param-row"><label>类型</label></div>
          <select id="be-kind" style="width:100%" ${existing ? 'disabled' : ''}>
            <option value="playbook" ${existing && existing.kind === 'script' ? '' : 'selected'}>剧本（从剧本库选择，可多选按序执行）</option>
            <option value="script" ${existing && existing.kind === 'script' ? 'selected' : ''}>自定义命令</option>
          </select>
          <div id="be-playbook-pane" style="display:${existing && existing.kind === 'script' ? 'none' : 'block'}">
            <div class="param-row"><label>搜索并选择剧本</label></div>
            <input id="be-search" type="text" placeholder="输入剧本名搜索…" style="width:100%">
            <ul id="be-pb-list" style="list-style:none;margin:6px 0;padding:0;max-height:180px;overflow:auto;border:1px solid var(--border);border-radius:var(--radius)"></ul>
            <div class="cfg-hint">已选（按点击顺序，即执行顺序）：</div>
            <div id="be-selected" style="display:flex;flex-wrap:wrap;gap:6px;margin-top:4px"></div>
          </div>
          <div id="be-script-pane" style="display:${existing && existing.kind === 'script' ? 'block' : 'none'}">
            <div class="param-row"><label>名称</label></div>
            <input id="be-name" type="text" value="${esc(existing ? existing.name : '')}" placeholder="如：清理大日志文件" style="width:100%">
            <div class="param-row"><label>命令内容</label></div>
            <textarea id="be-content" rows="6" placeholder="systemctl restart nginx" style="width:100%;font-family:var(--font-mono);font-size:var(--fs-xs)">${esc(existing && existing.kind === 'script' ? existing.content : '')}</textarea>
            <div class="param-row"><label>风险等级</label></div>
            <select id="be-risk" style="width:140px">
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
            </select>
          </div>
          <div style="display:flex;gap:16px;align-items:center;margin-top:10px;flex-wrap:wrap">
            <label style="font-size:var(--fs-xs);display:flex;align-items:center;gap:6px"><input type="checkbox" id="be-auto" ${existing && existing.auto_exec ? 'checked' : ''} style="accent-color:var(--accent)"> 告警自动执行</label>
            <label style="font-size:var(--fs-xs);display:flex;align-items:center;gap:6px">执行方式
              <select id="be-mode" style="width:auto">
                <option value="sequential">顺序串行</option>
                <option value="concurrent">并发执行</option>
              </select>
            </label>
          </div>
          <div class="cfg-hint">自动执行：告警首次触发或合并窗口重开时按所选方式自动运行；手动触发：在本告警详情中勾选执行。目标节点为告警所在节点。</div>
        </div>
        <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:14px">
          <button class="btn btn-ghost btn-sm" id="be-cancel">取消</button>
          <button class="btn btn-secondary btn-sm" id="be-save">保存</button>
        </div>
      </div>
    </div>`;
    scope.resources.overlay(editor);

    const pbList = editor.querySelector('#be-pb-list');
    const searchInput = editor.querySelector('#be-search');
    const selWrap = editor.querySelector('#be-selected');

    function renderPbList() {
      const q = (searchInput.value || '').toLowerCase();
      const items = playbooks.filter(p => !q || p.name.toLowerCase().includes(q) || (p.description || '').toLowerCase().includes(q));
      pbList.innerHTML = items.map(p => `
        <li style="padding:6px 10px;border-bottom:1px solid var(--border)">
          <label style="display:flex;align-items:center;gap:8px;font-size:var(--fs-xs);cursor:pointer">
            <input type="checkbox" class="be-pb-check" data-pbid="${esc(p.id)}" ${selected.includes(p.id) ? 'checked' : ''} style="accent-color:var(--accent)">
            <span style="font-weight:600">${esc(p.name)}</span>
            <span style="color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(p.category || '')} ${esc(p.description || '')}</span>
          </label>
        </li>`).join('') || '<li style="padding:10px;color:var(--muted);font-size:var(--fs-xs)">无匹配剧本</li>';
    }
    function renderSelected() {
      selWrap.innerHTML = selected.map((id, i) => {
        const pb = playbooks.find(x => x.id === id);
        return `<span style="display:inline-flex;align-items:center;gap:4px;padding:2px 8px;border:1px solid var(--border);border-radius:999px;font-size:var(--fs-xs);background:var(--bg)">
          <b style="color:var(--accent)">${i + 1}</b> ${esc(pb ? pb.name : id)}
          <button type="button" data-sel-up="${i}" ${i === 0 ? 'disabled' : ''} style="border:none;background:none;cursor:pointer;color:var(--muted)">↑</button>
          <button type="button" data-sel-del="${i}" style="border:none;background:none;cursor:pointer;color:var(--danger)">✕</button>
        </span>`;
      }).join('') || '<span class="cfg-hint">未选择</span>';
      selWrap.querySelectorAll('[data-sel-up]').forEach(btn => btn.addEventListener('click', () => {
        const i = parseInt(btn.dataset.selUp, 10);
        if (i > 0) { [selected[i - 1], selected[i]] = [selected[i], selected[i - 1]]; renderSelected(); }
      }));
      selWrap.querySelectorAll('[data-sel-del]').forEach(btn => btn.addEventListener('click', () => {
        selected.splice(parseInt(btn.dataset.selDel, 10), 1);
        renderSelected();
        renderPbList();
      }));
    }
    searchInput.addEventListener('input', renderPbList);
    pbList.addEventListener('change', (e) => {
      const cb = e.target.closest('.be-pb-check');
      if (!cb) return;
      const id = cb.dataset.pbid;
      if (cb.checked) { if (!selected.includes(id)) selected.push(id); }
      else selected = selected.filter(x => x !== id);
      renderSelected();
    });
    renderPbList();
    renderSelected();
    editor.querySelector('#be-risk').value = existing ? (existing.risk || 'medium') : 'medium';
    editor.querySelector('#be-mode').value = existing ? (existing.exec_mode || 'sequential') : 'sequential';

    const kindSel = editor.querySelector('#be-kind');
    kindSel.addEventListener('change', () => {
      editor.querySelector('#be-playbook-pane').style.display = kindSel.value === 'playbook' ? 'block' : 'none';
      editor.querySelector('#be-script-pane').style.display = kindSel.value === 'script' ? 'block' : 'none';
    });

    const close = () => editor.remove();
    editor.querySelector('#be-close').addEventListener('click', close);
    editor.querySelector('#be-cancel').addEventListener('click', close);
    editor.addEventListener('click', (e) => { if (e.target === editor) close(); });

    editor.querySelector('#be-save').addEventListener('click', async () => {
      const auto = editor.querySelector('#be-auto').checked;
      const mode = editor.querySelector('#be-mode').value;
      try {
        if (kindSel.value === 'playbook') {
          if (!selected.length) return alert('请从剧本库选择剧本');
          if (!isNew && selected.length > 1) return alert('编辑模式只能保留一个剧本，多选请删除后重新添加');
          if (isNew) {
            const baseSeq = await nextBindingSeq(alert.id);
            for (let i = 0; i < selected.length; i++) {
              const pb = playbooks.find(x => x.id === selected[i]);
              await api.createAlertBinding(alert.id, {
                kind: 'playbook', name: pb ? pb.name : selected[i], content: selected[i],
                risk: 'medium', auto_exec: auto, exec_mode: mode, seq: baseSeq + i,
              });
            }
          } else {
            await api.updateAlertBinding(existing.id, { content: selected[0], auto_exec: auto, exec_mode: mode });
          }
        } else {
          const name = editor.querySelector('#be-name').value.trim();
          const content = editor.querySelector('#be-content').value;
          if (!name) return alert('请填写名称');
          if (!content.trim()) return alert('请填写命令内容');
          const risk = editor.querySelector('#be-risk').value;
          if (isNew) {
            const seq = await nextBindingSeq(alert.id);
            await api.createAlertBinding(alert.id, { kind: 'script', name, content, risk, auto_exec: auto, exec_mode: mode, seq });
          } else {
            await api.updateAlertBinding(existing.id, { name, content, risk, auto_exec: auto, exec_mode: mode });
          }
        }
        close();
        parentOverlay.remove();
        openDetail(alert.id);
      } catch (err) { alert('保存失败: ' + (err.message || err)); }
    });
  }

  function scopeText(at) {
    const n = (at.scope_nodes || '').split(',').filter(x => x.trim()).length;
    const g = (at.scope_groups || '').split(',').filter(x => x.trim()).length;
    if (!n && !g) return '全部节点';
    const parts = [];
    if (n) parts.push(`节点${n}`);
    if (g) parts.push(`分组${g}`);
    return parts.join('+');
  }

  // ---------- 触发范围编辑（admin） ----------

  function openScopeEditor(at) {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay open';
    overlay.innerHTML = `<div class="modal" style="max-width:560px">
      <div class="modal-header"><h3>触发范围 · ${esc(at.name)}</h3>
        <button class="btn btn-ghost btn-icon" id="sc-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <div class="param-group">
          <div class="param-row"><label>指定节点 ID（逗号分隔）</label></div>
          <input id="sc-nodes" type="text" value="${esc(at.scope_nodes || '')}" placeholder="如 web-01, db-02" style="width:100%">
          <div class="param-row"><label>指定分组（逗号分隔）</label></div>
          <input id="sc-groups" type="text" value="${esc(at.scope_groups || '')}" placeholder="如 web, db" style="width:100%">
          <div class="cfg-hint">两者留空 = 对全部节点生效；任一命中即生效。注意：缩小范围后，范围外已有告警不会自动恢复，需手动解决。</div>
        </div>
        <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:14px">
          <button class="btn btn-ghost btn-sm" id="sc-cancel">取消</button>
          <button class="btn btn-secondary btn-sm" id="sc-save">保存</button>
        </div>
      </div>
    </div>`;
    scope.resources.overlay(overlay);
    const close = () => overlay.remove();
    overlay.querySelector('#sc-close').addEventListener('click', close);
    overlay.querySelector('#sc-cancel').addEventListener('click', close);
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); });
    overlay.querySelector('#sc-save').addEventListener('click', async () => {
      try {
        await api.updateAlertType(at.id, {
          ...at,
          scope_nodes: overlay.querySelector('#sc-nodes').value.trim(),
          scope_groups: overlay.querySelector('#sc-groups').value.trim(),
        });
        close();
        renderConfig();
      } catch (err) { alert('保存失败: ' + (err.message || err)); }
    });
  }

  // ---------- 告警规则调试对话框（admin） ----------

  async function openAlertDebugModal(at) {
    let nodes = [];
    try { nodes = await api.nodesAll(); } catch { /* 节点列表不可用不阻塞 */ }

    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay open';
    overlay.innerHTML = `<div class="modal" style="max-width:640px;max-height:84vh;overflow:auto">
      <div class="modal-header"><h3>调试 · ${esc(at.name)}</h3>
        <button class="btn btn-ghost btn-icon" id="dbg-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <div class="param-row"><label>目标节点</label></div>
        <select id="dbg-node" style="width:100%">
          ${nodes.map(n => `<option value="${esc(n.id)}">${esc(n.id)}${n.name && n.name !== n.id ? '（' + esc(n.name) + '）' : ''}</option>`).join('')}
        </select>
        <div class="cfg-hint" style="margin:6px 0 10px">立即执行一次检查命令（不落库、不发通知），展示输出与阈值判定。</div>
        <div style="display:flex;gap:8px;margin-bottom:10px">
          <button class="btn btn-secondary btn-sm" id="dbg-run">执行一次</button>
          <span id="dbg-status" class="cfg-hint"></span>
        </div>
        <div id="dbg-result"></div>
      </div>
    </div>`;
    scope.resources.overlay(overlay);

    // 默认选中范围内第一个节点
    const scopeFirst = (at.scope_nodes || '').split(',').map(x => x.trim()).filter(Boolean)[0];
    if (scopeFirst) {
      const sel = overlay.querySelector('#dbg-node');
      if ([...sel.options].some(o => o.value === scopeFirst)) sel.value = scopeFirst;
    }
    const close = () => overlay.remove();
    overlay.querySelector('#dbg-close').addEventListener('click', close);
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); });

    overlay.querySelector('#dbg-run').addEventListener('click', async () => {
      const btn = overlay.querySelector('#dbg-run');
      const status = overlay.querySelector('#dbg-status');
      const area = overlay.querySelector('#dbg-result');
      btn.disabled = true;
      status.textContent = '执行中…';
      area.innerHTML = '';
      try {
        const res = (await api.testAlertType(at.id, { node_id: overlay.querySelector('#dbg-node').value })).item;
        status.textContent = `耗时 ${res.duration_ms} ms`;
        const ST = { pending: '排队中', running: '执行中', success: '成功', failed: '失败' };
        area.innerHTML = `
          <div class="param-row"><label>检查命令</label></div>
          <pre style="margin:4px 0 8px;padding:8px;background:var(--bg);border-radius:var(--radius);font-family:var(--font-mono);font-size:var(--fs-xs);white-space:pre-wrap;word-break:break-all">${esc(res.command)}</pre>
          <div class="param-row"><label>输出（退出码 ${res.exit_code}）</label></div>
          <pre style="margin:4px 0 8px;padding:8px;background:var(--bg);border-radius:var(--radius);font-family:var(--font-mono);font-size:var(--fs-xs);white-space:pre-wrap;word-break:break-all;max-height:180px;overflow:auto">${esc(res.output || '（空）')}</pre>
          ${res.parse_err
            ? `<div style="color:var(--danger);font-size:var(--fs-sm)">解析失败：${esc(res.parse_err)}</div>`
            : `<div style="font-size:var(--fs-sm);margin-bottom:6px">解析值 <strong>${res.value}</strong> · 条件 <code>${esc(res.metric)} ${esc(res.op)} ${res.threshold}</code></div>
               <div style="font-size:var(--fs-sm);font-weight:600;color:${res.would_trigger ? 'var(--danger)' : 'var(--success)'}">${res.would_trigger ? '⚠ 满足阈值条件——按当前配置会触发告警' : '✓ 未满足阈值条件——不会触发告警'}</div>`}
        `;
      } catch (err) { alert('调试失败: ' + (err.message || err)); }
      btn.disabled = false;
      status.textContent = '';
    });
  }

  // ---------- 自定义告警配置表单（admin）：检查命令 + 阈值 + 处置指令 ----------

  async function openCustomAlertModal(onSaved) {
    let playbooks = [];
    try { playbooks = ((await api.playbooks()).data || []).filter(p => p.file_exists !== false); } catch { /* 剧本库不可用不阻塞 */ }

    const editor = document.createElement('div');
    editor.className = 'modal-overlay open';
    editor.innerHTML = `<div class="modal" style="max-width:680px;max-height:86vh;overflow:auto">
      <div class="modal-header"><h3>添加自定义告警</h3>
        <button class="btn btn-ghost btn-icon" id="ca-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <div class="mon-grid">
          <div class="mon-label">名称</div>
          <div class="mon-control"><input id="ca-name" type="text" placeholder="如：Kafka 消费积压"></div>
          <div class="mon-hint">告警列表中显示的名称</div>

          <div class="mon-label">级别</div>
          <div class="mon-control">
            <select id="ca-sev" style="width:100%">
              <option value="warn">警告（warn）</option>
              <option value="critical">紧急（critical）</option>
              <option value="info">提示（info）</option>
            </select>
          </div>
          <div class="mon-hint">触发后的告警级别</div>

          <div class="mon-label">检查命令</div>
          <div class="mon-control"><textarea id="ca-cmd" rows="3" placeholder="如：kafka-consumer-groups --describe --group order | awk '{sum+=\$5} END {print sum}'" style="font-family:var(--font-mono)"></textarea></div>
          <div class="mon-hint">每轮采集（60 秒）在各节点通过 SSH 执行，超时 30 秒</div>

          <div class="mon-label">解析方式</div>
          <div class="mon-control">
            <select id="ca-mode" style="width:100%">
              <option value="value">数值输出（输出即指标值）</option>
              <option value="regex">正则捕获（捕获组 1 为数值）</option>
              <option value="exit_code">退出码（0 正常 / 非 0 异常）</option>
            </select>
          </div>
          <div class="mon-hint">把命令输出转成数值指标</div>

          <div class="mon-label" id="ca-pattern-label" style="display:none">正则表达式</div>
          <div class="mon-control" id="ca-pattern-wrap" style="display:none"><input id="ca-pattern" type="text" placeholder="如：lag=(\\d+)" style="font-family:var(--font-mono)"></div>
          <div class="mon-hint" id="ca-pattern-hint" style="display:none">第一个捕获组将被解析为数值</div>

          <div class="mon-label">触发条件</div>
          <div class="mon-control" style="display:flex;gap:8px">
            <select id="ca-op" style="width:90px">
              <option value=">">&gt;</option>
              <option value="<">&lt;</option>
              <option value=">=">&gt;=</option>
              <option value="<=">&lt;=</option>
              <option value="==">==</option>
              <option value="!=">!=</option>
            </select>
            <input id="ca-value" type="number" placeholder="阈值" style="flex:1">
          </div>
          <div class="mon-hint">指标值满足条件时计入触发</div>

          <div class="mon-label">持续次数</div>
          <div class="mon-control"><input id="ca-duration" type="number" value="2" min="1" style="width:100%"></div>
          <div class="mon-hint">连续满足 N 次才触发（防抖动）</div>

          <div class="mon-label">生效范围</div>
          <div class="mon-control" style="display:flex;gap:8px">
            <input id="ca-scope-nodes" type="text" placeholder="节点 ID，逗号分隔" style="flex:1">
            <input id="ca-scope-groups" type="text" placeholder="分组，逗号分隔" style="flex:1">
          </div>
          <div class="mon-hint">两者留空 = 对全部节点生效；任一命中即触发。后续可在规则表「范围」列调整</div>

          <div class="mon-label">处置指令</div>
          <div class="mon-control">
            <select id="ca-remedy-kind" style="width:100%">
              <option value="">暂不绑定</option>
              <option value="playbook">绑定剧本</option>
              <option value="script">自定义命令</option>
            </select>
          </div>
          <div class="mon-hint">触发后可在告警详情中一键执行</div>

          <div class="mon-label" id="ca-pb-label" style="display:none">选择剧本</div>
          <div class="mon-control" id="ca-pb-wrap" style="display:none">
            <select id="ca-playbook" style="width:100%">
              <option value="">— 选择剧本 —</option>
              ${playbooks.map(pb => `<option value="${esc(pb.id)}">${esc(pb.name)}</option>`).join('')}
            </select>
          </div>
          <div class="mon-hint" id="ca-pb-hint" style="display:none"></div>

          <div class="mon-label" id="ca-script-label" style="display:none">处置命令</div>
          <div class="mon-control" id="ca-script-wrap" style="display:none"><textarea id="ca-script" rows="3" style="font-family:var(--font-mono)"></textarea></div>
          <div class="mon-hint" id="ca-script-hint" style="display:none"></div>

          <div class="mon-label">自动执行</div>
          <div class="mon-control"><label class="mon-check"><input type="checkbox" id="ca-auto"> 开启</label></div>
          <div class="mon-hint">告警触发时自动执行所绑定的处置命令（剧本类仅创建对策，需手动运行）</div>
        </div>
        <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:16px">
          <button class="btn btn-ghost btn-sm" id="ca-cancel">取消</button>
          <button class="btn btn-primary btn-sm" id="ca-save">创建</button>
        </div>
      </div>
    </div>`;
    scope.resources.overlay(editor);

    const modeSel = editor.querySelector('#ca-mode');
    const toggle = (sel, show) => { [sel + '-label', sel + '-wrap', sel + '-hint'].forEach(id => {
      const el = editor.querySelector('#' + id);
      if (el) el.style.display = show ? '' : 'none';
    }); };
    modeSel.addEventListener('change', () => toggle('ca-pattern', modeSel.value === 'regex'));
    const remedySel = editor.querySelector('#ca-remedy-kind');
    remedySel.addEventListener('change', () => {
      toggle('ca-pb', remedySel.value === 'playbook');
      toggle('ca-script', remedySel.value === 'script');
    });

    const close = () => editor.remove();
    editor.querySelector('#ca-close').addEventListener('click', close);
    editor.querySelector('#ca-cancel').addEventListener('click', close);
    editor.addEventListener('click', (e) => { if (e.target === editor) close(); });

    editor.querySelector('#ca-save').addEventListener('click', async () => {
      const name = editor.querySelector('#ca-name').value.trim();
      const cmd = editor.querySelector('#ca-cmd').value.trim();
      const mode = modeSel.value;
      const op = editor.querySelector('#ca-op').value;
      const value = parseFloat(editor.querySelector('#ca-value').value);
      const duration = parseInt(editor.querySelector('#ca-duration').value, 10) || 1;
      if (!name) return alert('请填写名称');
      if (!cmd) return alert('请填写检查命令');
      if (isNaN(value)) return alert('请填写阈值');
      const id = 'OWL-CUS-' + Date.now().toString(36);
      try {
        await api.createAlertType({
          id, name, category: 'custom',
          default_severity: editor.querySelector('#ca-sev').value,
          default_params: { metric: owlMetric(id), op, value, duration },
          enabled: true, notifiable: true, builtin: false,
          check_cmd: cmd, check_mode: mode,
          check_pattern: mode === 'regex' ? editor.querySelector('#ca-pattern').value.trim() : '',
          scope_nodes: editor.querySelector('#ca-scope-nodes').value.trim(),
          scope_groups: editor.querySelector('#ca-scope-groups').value.trim(),
        });
        // 处置指令绑定
        const rk = remedySel.value;
        if (rk === 'playbook') {
          const pbid = editor.querySelector('#ca-playbook').value;
          const pb = playbooks.find(x => x.id === pbid);
          if (!pbid) return alert('请选择剧本');
          await api.createRemedy({ id: 'RM-' + Date.now(), alert_type_id: id,
            name: (pb ? pb.name : pbid) + '（自动处置）', kind: 'playbook', content: pbid,
            risk: 'medium', source: 'user', reviewed: true,
            auto_approve: editor.querySelector('#ca-auto').checked });
        } else if (rk === 'script') {
          const content = editor.querySelector('#ca-script').value;
          if (!content.trim()) return alert('请填写处置命令');
          await api.createRemedy({ id: 'RM-' + Date.now(), alert_type_id: id,
            name: name + '（自动处置）', kind: 'script', content,
            risk: 'medium', source: 'user', reviewed: true,
            auto_approve: editor.querySelector('#ca-auto').checked });
        }
        close();
        if (onSaved) onSaved();
      } catch (err) { alert('创建失败: ' + (err.message || err)); }
    });
  }

  function owlMetric(id) { return 'custom.' + id.toLowerCase(); }

  // ---------- 监控配置（admin）：告警类型的处置指令管理入口 ----------

  async function openRemedyManager(at) {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay open';
    async function render() {
      let items = [];
      try { items = (await api.remedies(at.id)).items || []; } catch {}
      overlay.innerHTML = `<div class="modal" style="max-width:720px;max-height:84vh;overflow:auto">
        <div class="modal-header"><h3>处置指令 · ${esc(at.name)} <code style="font-size:var(--fs-xs)">${esc(at.id)}</code></h3>
          <button class="btn btn-ghost btn-icon" id="rmgr-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
        <div class="modal-body">
          ${items.length ? `<ul style="list-style:none;margin:0;padding:0">${items.map(r => `
            <li class="remedy-row">
              <div class="rm-head">
                <strong style="font-size:var(--fs-sm);flex:1">${esc(r.name)}</strong>
                <span style="font-size:var(--fs-xs);color:var(--muted)">${REMEDY_KIND_TEXT[r.kind] || esc(r.kind)} · 风险 ${esc(r.risk)} · ${r.source === 'user' ? '用户自定义' : r.source === 'builtin' ? '内置' : 'AI 生成'}</span>
                <button class="btn btn-ghost btn-sm" data-mgr-edit="${esc(r.id)}">编辑</button>
              </div>
              <pre class="rm-content">${esc(r.kind === 'playbook' ? '剧本: ' + r.content : r.content)}</pre>
            </li>`).join('')}</ul>` : '<p class="cfg-hint">该告警类型暂无处置指令</p>'}
          <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:12px">
            <button class="btn btn-secondary btn-sm" id="rmgr-add">+ 添加指令</button>
          </div>
        </div>
      </div>`;
      overlay.querySelector('#rmgr-close').addEventListener('click', () => overlay.remove());
      overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
      overlay.querySelector('#rmgr-add').addEventListener('click', () =>
        openRemedyEditor({ alertTypeId: at.id, existing: null, onSaved: render }));
      overlay.querySelectorAll('[data-mgr-edit]').forEach(btn => btn.addEventListener('click', () => {
        const r = items.find(x => x.id === btn.dataset.mgrEdit);
        if (r) openRemedyEditor({ alertTypeId: at.id, existing: r, onSaved: render });
      }));
    }
    scope.resources.overlay(overlay);
    await render();
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
      <section class="cfg-section">
        <h3>全局静默</h3>
        <div class="cfg-form-row">
          <input id="cfg-silence" type="datetime-local" value="${silenceUntil ? new Date(silenceUntil * 1000 - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ''}" style="width:220px">
          <button class="btn btn-secondary btn-sm" id="cfg-silence-set">设置静默</button>
          <button class="btn btn-ghost btn-sm" id="cfg-silence-clear" ${silenceUntil ? '' : 'disabled'}>取消静默</button>
          <span class="cfg-hint">${silenceUntil ? '静默至 ' + new Date(silenceUntil * 1000).toLocaleString('zh-CN', { hour12: false }) : '未静默'}</span>
        </div>
      </section>

      <section class="cfg-section">
        <h3 style="display:flex;align-items:center;gap:8px">告警类型（阈值 / 开关 / 自动执行放行）
          <span style="flex:1"></span>
          <button class="btn btn-secondary btn-sm" id="at-custom-add">+ 自定义告警</button>
        </h3>
        <div style="overflow-x:auto"><table class="table"><thead><tr><th>ID</th><th>名称</th><th>级别</th><th>阈值</th><th>持续</th><th>范围</th><th>调试</th><th>启用</th><th>自动放行</th><th colspan="2"></th></tr></thead>
        <tbody>${types.map(at => `
          <tr data-at="${esc(at.id)}">
            <td><code>${esc(at.id)}</code></td>
            <td>${esc(at.name)}</td>
            <td>${esc(at.default_severity)}</td>
            <td><input class="at-val" style="width:70px" value="${at.default_params.per_core > 0 ? at.default_params.per_core : at.default_params.value}" data-kind="${at.default_params.per_core > 0 ? 'per_core' : 'value'}">${at.default_params.per_core > 0 ? '×核' : ''}</td>
            <td><input class="at-dur" style="width:50px" value="${at.default_params.duration || 0}"> 次</td>
            <td><button class="btn btn-ghost btn-sm at-scope" data-at-scope="${esc(at.id)}" title="点击编辑触发范围">${esc(scopeText(at))}</button></td>
            <td><button class="btn btn-ghost btn-sm at-debug" data-at-debug="${esc(at.id)}" ${at.check_cmd ? '' : 'disabled title="内置规则随采集自动评估"'}>调试</button></td>
            <td><input type="checkbox" class="at-enabled" ${at.enabled ? 'checked' : ''}></td>
            <td><input type="checkbox" class="at-auto" ${at.auto_approve ? 'checked' : ''}></td>
            <td><div style="display:flex;gap:4px"><button class="btn btn-ghost btn-sm at-remedies">指令</button><button class="btn btn-secondary btn-sm at-save">保存</button></div></td>
            ${at.builtin ? '' : `<td><button class="btn btn-ghost btn-sm at-del" data-at-del="${esc(at.id)}" style="color:var(--danger)">删除</button></td>`}
          </tr>`).join('')}</tbody></table></div>
      </section>

      <section class="cfg-section">
        <h3>通知渠道</h3>
        <div id="cfg-channels" style="margin-bottom:10px">
          ${channels.length ? channels.map(ch => `
            <div class="channel-row">
              <span class="ch-name">${esc(ch.name)}</span>
              <span class="ch-meta">${esc(ch.kind)} · 门槛 ${esc(ch.severity_min)} · ${ch.alert_types ? '类型: ' + esc(ch.alert_types) : '全部类型'}</span>
              <span class="ch-state ${ch.enabled ? 'on' : 'off'}">${ch.enabled ? '启用' : '停用'}</span>
              <span style="flex:1"></span>
              <button class="btn btn-ghost btn-sm" data-toggle-ch="${esc(ch.id)}">${ch.enabled ? '停用' : '启用'}</button>
              <button class="btn btn-ghost btn-sm" data-test-ch="${esc(ch.id)}">测试</button>
              <button class="btn btn-ghost btn-sm" data-del-ch="${esc(ch.id)}">删除</button>
            </div>`).join('') : '<p class="cfg-hint">暂无渠道</p>'}
        </div>
        <div class="cfg-form-row">
          <input id="ch-name" placeholder="渠道名" style="width:110px">
          <select id="ch-kind" style="width:100px"><option value="webhook">Webhook</option><option value="email">邮件</option></select>
          <input id="ch-sev" placeholder="门槛(warn)" style="width:90px" value="warn">
          <input id="ch-types" placeholder="告警ID(空=全部,逗号分隔)" style="width:190px">
          <input id="ch-url" placeholder="Webhook URL / SMTP host:port" style="width:210px">
          <select id="ch-enc" style="width:110px" title="邮件加密方式">
            <option value="ssl">SSL(465)</option>
            <option value="starttls">STARTTLS(587)</option>
            <option value="none">无加密(25)</option>
          </select>
          <input id="ch-user" placeholder="账号(可选)" style="width:100px">
          <input id="ch-pass" type="password" placeholder="密码(可选)" style="width:100px">
          <input id="ch-from" placeholder="发件人(邮件)" style="width:110px">
          <input id="ch-to" placeholder="收件人,逗号分隔(邮件)" style="width:180px">
          <button class="btn btn-secondary btn-sm" id="ch-add">添加</button>
        </div>
        <p class="cfg-hint" style="margin-top:6px">邮件加密：465 端口选 SSL，587 端口选 STARTTLS，25 端口选无加密。添加后点「测试」验证真实投递。</p>
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
        try { await api.updateAlertType(id, at); btn.textContent = '✓ 已保存'; scope.resources.setTimeout(() => { btn.textContent = '保存'; }, 1500); } catch (e) { alert('保存失败: ' + (e.message || e)); }
      });
    });

    // 告警类型：调试执行
    container.querySelectorAll('.at-debug').forEach(btn => {
      btn.addEventListener('click', () => {
        const at = types.find(x => x.id === btn.dataset.atDebug);
        if (at) openAlertDebugModal(at);
      });
    });

    // 告警类型：触发范围编辑
    container.querySelectorAll('.at-scope').forEach(btn => {
      btn.addEventListener('click', () => {
        const at = types.find(x => x.id === btn.dataset.atScope);
        if (at) openScopeEditor(at);
      });
    });

    // 告警类型：删除自定义类型
    container.querySelectorAll('[data-at-del]').forEach(btn => {
      btn.addEventListener('click', async () => {
        if (!confirm('删除该自定义告警类型？其处置指令将一并删除。')) return;
        try { await api.deleteAlertType(btn.dataset.atDel); renderConfig(); }
        catch (e) { alert('删除失败: ' + (e.message || e)); }
      });
    });

    // 自定义告警表单
    document.getElementById('at-custom-add')?.addEventListener('click', () => openCustomAlertModal(renderConfig));

    // 告警类型：处置指令管理
    container.querySelectorAll('.at-remedies').forEach(btn => {
      btn.addEventListener('click', () => {
        const at = types.find(x => x.id === btn.closest('tr').dataset.at);
        if (at) openRemedyManager(at);
      });
    });

    // 通知渠道：测试/删除/启停
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
    container.querySelectorAll('[data-toggle-ch]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const ch = channels.find(x => x.id === btn.dataset.toggleCh);
        if (!ch) return;
        try {
          await api.updateNotifyChannel(ch.id, { ...ch, enabled: !ch.enabled });
          renderConfig();
        } catch (e) { alert('操作失败: ' + (e.message || e)); }
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
          smtp_host: hp[0] || '', smtp_port: parseInt(hp[1]) || 0,
          encryption: document.getElementById('ch-enc').value,
          username: document.getElementById('ch-user').value.trim(),
          password: document.getElementById('ch-pass').value,
          from: document.getElementById('ch-from').value.trim(),
          to: document.getElementById('ch-to').value.split(',').map(x => x.trim()).filter(Boolean),
        } };
      }
      try { await api.createNotifyChannel(ch); renderConfig(); } catch (e) { alert('添加失败: ' + (e.message || e)); }
    });
  }

  // ---------- 初始化 ----------

  renderView();
  loadPanel();
  api.alertTypes().then(res => {
    state.types = res.items || [];
    if (mode === 'list') renderFilterBar();
  }).catch(() => {});
}
