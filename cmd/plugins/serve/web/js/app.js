import { api } from './api.js';
import { createPageScope } from './pagescope.js';
import { createTabStore } from './tabs.js';
import { renderLogin } from './pages/login.js';
import { renderDashboard } from './pages/dashboard.js';
import { renderExec } from './pages/exec.js';
import { renderFiles } from './pages/files.js';
import { renderAI } from './pages/ai.js';
import { renderNodes } from './pages/nodes.js';
import { renderNodeDetail } from './pages/node.js';
import { renderTasks } from './pages/tasks.js';
import { renderSettings } from './pages/settings.js';
import { renderUsers } from './pages/users.js';
import { renderPlaybooks } from './pages/playbooks.js';
import { renderHistory } from './pages/history.js';
import { renderAlerts } from './pages/alerts.js';

let currentCleanup = null;
let currentScope = null;
let shellRendered = false;
let currentViewId = null;

const VIEW_TITLES = {
  dashboard: '仪表盘', nodes: '节点管理', exec: '命令执行',
  playbooks: '剧本管理', files: '文件传输', ai: 'AI 助手',
  history: '任务历史', settings: '系统设置', users: '用户管理', alerts: '告警中心'
};

const PANEL_TITLES = {
  nodes: '节点分组', exec: '节点选择',
  playbooks: '剧本分类', files: '节点选择', ai: '对话上下文',
  history: '过滤条件', settings: '系统配置', users: '用户角色', alerts: '告警过滤'
};

const NAV_ITEMS = [
  { id: 'dashboard', icon: 'dashboard', label: '仪表盘' },
  { id: 'alerts', icon: 'bell', label: '告警中心' },
  { id: 'nodes', icon: 'nodes', label: '节点管理' },
  { id: 'exec', icon: 'terminal', label: '命令执行' },
  { id: 'playbooks', icon: 'scroll', label: '剧本管理' },
  { id: 'files', icon: 'upload', label: '文件传输' },
  { id: 'ai', icon: 'brain', label: 'AI 助手' },
  { id: 'history', icon: 'clock', label: '任务历史' }
];

const NAV_BOTTOM = [
  { id: 'settings', icon: 'settings', label: '系统设置' },
  { id: 'users', icon: 'users', label: '用户管理' }
];

const VIEW_ICONS = {};
[...NAV_ITEMS, ...NAV_BOTTOM].forEach(it => { VIEW_ICONS[it.id] = it.icon; });

// ---- 标签（M1：单挂载重建式）----
// 一次只挂载一个标签的页面；切标签 = 当前页状态写入标签快照 → 释放 → 按目标标签路由重挂载。
const tabs = createTabStore();
let currentPageTabId = null;   // 当前挂载页面对应的标签（页面快照写回它）
let tabMenuEl = null;
let dragTabId = null;

function routeForView(viewId) {
  return viewId === 'dashboard' ? '/' : '/' + viewId;
}

// 路由 → 标签归属：详情类路由归到它的列表页，标题用详情名
function routeInfo(path) {
  const p = String(path || '/').split('?')[0].replace(/\/+$/, '') || '/';
  if (p === '/') return { view: 'dashboard', title: '仪表盘', icon: VIEW_ICONS.dashboard };
  if (p.startsWith('/nodes/')) return { view: 'nodes', title: '节点详情', icon: VIEW_ICONS.nodes };
  if (p.startsWith('/terminal/')) return { view: 'nodes', title: '终端', icon: VIEW_ICONS.nodes };
  if (p.startsWith('/sftp/')) return { view: 'nodes', title: '文件管理', icon: VIEW_ICONS.nodes };
  if (p.startsWith('/tasks/')) return { view: 'history', title: '任务详情', icon: VIEW_ICONS.history };
  const seg = p.slice(1);
  if (VIEW_TITLES[seg]) {
    const q = new URLSearchParams(String(path).split('?')[1] || '');
    let suffix = '';
    if (seg === 'nodes' && q.get('group')) suffix += ' · ' + q.get('group');
    if (seg === 'playbooks' && q.get('cat')) suffix += ' · ' + q.get('cat');
    if (q.get('q')) suffix += ' · "' + q.get('q') + '"';
    return { view: seg, title: VIEW_TITLES[seg] + suffix, icon: VIEW_ICONS[seg] || '' };
  }
  return { view: 'dashboard', title: '仪表盘', icon: VIEW_ICONS.dashboard };
}

// 路由上下文（?group= / ?cat= / ?q=）：让「多层子菜单」的选择进 URL——
// 可深链接、刷新不丢、标签标题能看出层级，也便于区分同视图的多个标签。
const routeContext = {
  get() {
    const q = new URLSearchParams(location.search);
    const out = {};
    q.forEach((v, k) => { out[k] = v; });
    return out;
  },
  // 合并写回 URL；replaceState 避免筛选操作污染历史
  set(patch, opts = {}) {
    const q = new URLSearchParams(location.search);
    for (const [k, v] of Object.entries(patch || {})) {
      if (v === '' || v === null || v === undefined) q.delete(k);
      else q.set(k, String(v));
    }
    const search = q.toString();
    const path = location.pathname + (search ? '?' + search : '');
    history.replaceState(null, '', path);
    const info = routeInfo(path);
    tabs.updateActive({ route: path, title: opts.title || info.title });
  },
};

// 在「新标签」打开任意路由（中键点击列表行、右键菜单等）；超上限时退回当前标签
function openPathInNewTab(path) {
  if (tabs.tabs.length >= tabs.limit) {
    showTabHint('标签上限 ' + tabs.limit + ' 个，已在当前标签打开');
    navigate(path);
    return null;
  }
  const info = routeInfo(path);
  const t = tabs.create(info.view, path, info);
  activateTab(t.id);
  return t;
}

// 面板归属当前页面：切标签/切页时先清空，页面需要时用 scope.panel 自己填，
// 避免详情页还留着上一页的分组面板（多标签下尤其误导）。
function makePanel() {
  return {
    setContent(html) {
      const list = document.getElementById('panelList');
      if (list) list.innerHTML = html;
    },
    setTitle(title) { shell.setPanelTitle(title); },
    reset() {
      const list = document.getElementById('panelList');
      if (list) list.innerHTML = '';
    },
  };
}

// 启动时对齐「持久化标签集合」与「当前 URL」：URL 优先（深链接落在激活标签上）
function bootTabs() {
  tabs.restore();
  const path = location.pathname + location.search;
  const info = routeInfo(path);
  const active = tabs.active();
  if (!active) {
    const t = tabs.create(info.view, path, info);
    tabs.activate(t.id);
  } else if (!tabs.tabs.some(t => t.route === path)) {
    tabs.update(active.id, { view: info.view, route: path, title: info.title, icon: info.icon });
  }
}

// 每一次路由变化都把当前 URL 记到激活标签上（标题由 switchView / setViewMetadata 补）
function syncActiveTabRoute() {
  const tab = tabs.active();
  if (!tab) return;
  const path = location.pathname + location.search;
  if (tab.route === path) return;
  const info = routeInfo(path);
  tabs.update(tab.id, { route: path, view: info.view, icon: info.icon });
}

function activateTab(id) {
  const tab = tabs.get(id);
  if (!tab) return;
  if (id === tabs.activeId) { renderTabbar(); return; }
  tabs.activate(id);
  history.pushState(null, '', tab.route || routeForView(tab.view));
  router();
}

function closeTab(id) {
  if (!tabs.get(id)) return;
  const { wasActive } = tabs.close(id);
  tabs.ensureOne('dashboard', '/');
  if (!wasActive) return;
  const next = tabs.active();
  history.pushState(null, '', (next && next.route) || '/');
  router();
}

function closeTabMenu() {
  if (tabMenuEl) { tabMenuEl.remove(); tabMenuEl = null; }
}

function openTabMenu(x, y, id) {
  closeTabMenu();
  const el = document.createElement('div');
  el.className = 'tab-menu';
  el.id = 'tab-menu';
  el.innerHTML = `
    <button data-act="close">关闭</button>
    <button data-act="others">关闭其他</button>
    <button data-act="right">关闭右侧</button>
    <button data-act="all">全部关闭</button>`;
  el.style.left = Math.max(8, x) + 'px';
  el.style.top = (y + 4) + 'px';
  document.body.appendChild(el);
  tabMenuEl = el;
  const keepAlive = (keepId) => {
    if (tabs.activeId !== keepId) {
      tabs.activate(keepId);
      const t = tabs.get(keepId);
      history.pushState(null, '', (t && t.route) || '/');
      router();
    }
  };
  el.addEventListener('click', (e) => {
    const act = e.target.closest('button') && e.target.closest('button').dataset.act;
    if (!act) return;
    closeTabMenu();
    if (act === 'close') closeTab(id);
    else if (act === 'others') { tabs.closeOthers(id); keepAlive(id); }
    else if (act === 'right') { tabs.closeRight(id); keepAlive(id); }
    else if (act === 'all') { tabs.closeAll(); tabs.ensureOne('dashboard', '/'); navigate('/'); }
  });
  setTimeout(() => document.addEventListener('click', closeTabMenu, { once: true }), 0);
}

function nextTab(delta) {
  const list = tabs.tabs;
  if (list.length < 2) return;
  const idx = list.findIndex(t => t.id === tabs.activeId);
  const next = list[(idx + delta + list.length) % list.length];
  if (next) activateTab(next.id);
}

function showTabHint(msg) {
  const bar = document.getElementById('tabbar');
  if (!bar) return;
  let hint = document.getElementById('tab-hint');
  if (!hint) {
    hint = document.createElement('span');
    hint.id = 'tab-hint';
    hint.className = 'tab-hint';
    bar.appendChild(hint);
  }
  hint.textContent = msg;
  clearTimeout(showTabHint._t);
  showTabHint._t = setTimeout(() => { if (hint) hint.remove(); }, 2500);
}

function newTab() {
  if (tabs.tabs.length >= tabs.limit) {
    showTabHint(`标签上限 ${tabs.limit} 个，先关掉一个再用 Alt+T 新建`);
    return;
  }
  const t = tabs.create('dashboard', '/', { title: VIEW_TITLES.dashboard, icon: VIEW_ICONS.dashboard });
  activateTab(t.id);
}

// 导航入口：已打开的视图复用它的标签（保住里面的上下文），Ctrl/中键强制新标签
function openView(viewId, opts = {}) {
  const target = routeForView(viewId);
  const info = { title: VIEW_TITLES[viewId] || viewId, icon: VIEW_ICONS[viewId] || '' };
  if (opts.newTab) {
    if (tabs.tabs.length >= tabs.limit) {
      showTabHint(`标签上限 ${tabs.limit} 个，先关掉一个再新开`);
      navigate(target);
      return;
    }
    const t = tabs.create(viewId, target, info);
    activateTab(t.id);
    return;
  }
  const active = tabs.active();
  if (active && active.view === viewId) {
    if (location.pathname !== target) navigate(target);   // 详情页 → 回该视图列表
    return;
  }
  const existing = tabs.byView(viewId)[0];
  if (existing) { activateTab(existing.id); return; }
  if (tabs.tabs.length >= tabs.limit) {
    showTabHint(`标签上限 ${tabs.limit} 个，已在当前标签打开`);
    navigate(target);
    return;
  }
  const t = tabs.create(viewId, target, info);
  activateTab(t.id);
}

function renderTabbar() {
  const bar = document.getElementById('tabbar');
  if (!bar) return;
  const activeId = tabs.activeId;
  bar.innerHTML = tabs.tabs.map(t => `
    <div class="tab${t.id === activeId ? ' active' : ''}" data-tab-id="${esc(t.id)}" role="tab"
         aria-selected="${t.id === activeId}" draggable="true" title="${esc(t.title)} · ${esc(t.route || '')}">
      <svg class="tab-icon" aria-hidden="true"><use href="#icon-${t.icon || 'dashboard'}"/></svg>
      <span class="tab-title">${esc(t.title)}</span>
      <button type="button" class="tab-close" data-close="${esc(t.id)}" title="关闭标签 (Alt+W)" aria-label="关闭标签">×</button>
    </div>`).join('') +
    `<button type="button" class="tab-new" id="tab-new" title="新建标签 (Alt+T)" aria-label="新建标签">+</button>`;
}

function wireTabbar() {
  const bar = document.getElementById('tabbar');
  if (!bar) return;

  bar.addEventListener('click', (e) => {
    const closeBtn = e.target.closest('.tab-close');
    if (closeBtn) { closeTab(closeBtn.dataset.close); return; }
    if (e.target.closest('#tab-new')) { newTab(); return; }
    const tabEl = e.target.closest('.tab');
    if (tabEl) activateTab(tabEl.dataset.tabId);
  });

  bar.addEventListener('auxclick', (e) => {   // 中键关闭标签
    if (e.button !== 1) return;
    const tabEl = e.target.closest('.tab');
    if (tabEl) { e.preventDefault(); closeTab(tabEl.dataset.tabId); }
  });

  bar.addEventListener('contextmenu', (e) => {
    const tabEl = e.target.closest('.tab');
    if (!tabEl) return;
    e.preventDefault();
    openTabMenu(e.clientX, e.clientY, tabEl.dataset.tabId);
  });

  // 拖拽排序
  bar.addEventListener('dragstart', (e) => {
    const tabEl = e.target.closest('.tab');
    if (!tabEl) return;
    dragTabId = tabEl.dataset.tabId;
    tabEl.classList.add('dragging');
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
  });
  bar.addEventListener('dragend', (e) => {
    const tabEl = e.target.closest('.tab');
    if (tabEl) tabEl.classList.remove('dragging');
    dragTabId = null;
  });
  bar.addEventListener('dragover', (e) => { if (dragTabId) e.preventDefault(); });
  bar.addEventListener('drop', (e) => {
    if (!dragTabId) return;
    e.preventDefault();
    const tabEl = e.target.closest('.tab');
    if (!tabEl) return;
    const toIndex = tabs.tabs.findIndex(t => t.id === tabEl.dataset.tabId);
    if (toIndex >= 0) tabs.move(dragTabId, toIndex);
    dragTabId = null;
  });

  tabs.subscribe(renderTabbar);
  renderTabbar();
}

function esc(s) {
  return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]));
}

const shell = {
  setPanelContent(html) {
    const list = document.getElementById('panelList');
    if (list) list.innerHTML = html;
  },
  setPanelTitle(title) {
    const el = document.getElementById('panelTitle');
    if (el) {
      const span = el.querySelector('span');
      if (span) span.textContent = title;
      else el.textContent = title;
    }
  },
  setViewTitle(title) {
    const el = document.getElementById('viewTitle');
    if (el) el.textContent = title;
    const bc = document.getElementById('breadcrumbCurrent');
    if (bc) bc.textContent = title;
  }
};

// 每次导航开始时调用：把上一页的状态快照/滚动位置写回它所属的标签，释放上一页的
// cleanup 与资源作用域，再为即将挂载的页面建作用域（带上该标签上次的快照）。
function beginPage(name) {
  const outgoing = currentScope;
  const host = currentPageTabId ? tabs.get(currentPageTabId) : null;
  if (host && outgoing) {
    const snap = outgoing.takeSnapshot();
    if (snap) tabs.setSnapshot(host.id, outgoing.name, snap);
    const vc = document.querySelector('.view-container');
    if (vc) tabs.setScroll(host.id, outgoing.name, vc.scrollTop);
  }
  if (currentCleanup) {
    try { currentCleanup(); } catch (e) { console.warn('page cleanup failed:', e); }
    currentCleanup = null;
  }
  if (outgoing) outgoing.dispose();

  const tab = tabs.active();
  currentPageTabId = tab ? tab.id : null;
  const panel = makePanel();
  panel.reset();   // 面板随页面：先清空，页面需要时自己填
  currentScope = createPageScope(name, {
    snapshot: tab ? tab.snapshots[name] : null,
    tabId: currentPageTabId,     // 页面用它拼按标签命名的存储键
    panel,
    context: routeContext,
    openInNewTab: openPathInNewTab,
  });
  return currentScope;
}

function render(html, afterRender) {
  const container = document.querySelector('.view-container');
  const app = document.getElementById('app');
  if (container && shellRendered) {
    container.innerHTML = html;
  } else {
    app.innerHTML = html;
  }
  if (currentCleanup) currentCleanup();
  currentCleanup = null;
  if (afterRender) {
    const cleanup = afterRender();
    if (typeof cleanup === 'function') currentCleanup = cleanup;
  }
  // 恢复本标签在该页面的滚动位置（页面数据异步到达后高度变化，允许轻微偏移）
  const tab = currentPageTabId ? tabs.get(currentPageTabId) : null;
  const top = tab && currentScope ? (tab.scroll[currentScope.name] || 0) : 0;
  if (container && top) container.scrollTop = top;
}

function renderShell() {
  if (shellRendered) return;
  const user = getUser();
  if (!user) return;

  const app = document.getElementById('app');
  const initials = user.username ? user.username.charAt(0).toUpperCase() : 'U';
  const name = user.display_name || user.username || 'User';
  const role = user.role || 'viewer';

  app.innerHTML = `
<div class="app-shell">
  <nav class="nav-rail" aria-label="主导航">
    <div class="logo" aria-label="OWL Console 首页"><svg aria-hidden="true"><use href="#icon-owl"/></svg></div>
    <button class="nav-toggle" id="navToggle" title="展开导航" aria-label="展开导航">
      <svg width="16" height="16" aria-hidden="true"><use href="#icon-chevron-right"/></svg>
    </button>
    ${NAV_ITEMS.map(item => `
      <button class="nav-item" data-view="${item.id}" title="${item.label}" aria-label="${item.label}">
        <svg aria-hidden="true"><use href="#icon-${item.icon}"/></svg>
        <span class="nav-label">${item.label}</span>
      </button>
    `).join('')}
    <div class="nav-spacer"></div>
    ${NAV_BOTTOM.map(item => `
      <button class="nav-item" data-view="${item.id}" title="${item.label}" aria-label="${item.label}">
        <svg aria-hidden="true"><use href="#icon-${item.icon}"/></svg>
        <span class="nav-label">${item.label}</span>
      </button>
    `).join('')}
    <div class="nav-avatar" title="个人设置">${esc(initials)}</div>
    <a class="nav-signature" href="https://github.com/cangyunye/go-owl" target="_blank" rel="noopener" title="github.com/cangyunye/go-owl · cangyunye">
      <svg aria-hidden="true"><use href="#icon-git"/></svg>
      <span class="nav-signature-text">cangyunye/go-owl</span>
    </a>
  </nav>
  <aside class="panel" id="sidePanel">
    <div class="panel-header" id="panelTitle">
      <span>导航</span>
    </div>
    <div class="panel-list" id="panelList"></div>
  </aside>
  <button class="panel-toggle" id="panelToggle" title="折叠面板" aria-label="折叠面板">
    <svg width="14" height="14" aria-hidden="true"><use href="#icon-chevron-right"/></svg>
  </button>
  <main class="main-area">
    <header class="topbar">
      <div class="view-title" id="viewTitle">仪表盘</div>
      <div class="breadcrumb" id="breadcrumb">
        <span>运维中心</span>
        <svg width="12" height="12" aria-hidden="true" style="color:var(--muted)"><use href="#icon-chevron-right"/></svg>
        <span id="breadcrumbCurrent">仪表盘</span>
      </div>
      <div class="topbar-spacer"></div>
      <div class="topbar-stats" id="topbarStats">
        <div class="topbar-stat">
          <span class="dot-indicator online"></span>
          <span class="num" id="statOnline">0</span>
          <span style="font-size:11px;color:var(--muted)">在线</span>
        </div>
        <div class="topbar-stat">
          <span class="dot-indicator offline"></span>
          <span class="num" id="statOffline">0</span>
          <span style="font-size:11px;color:var(--muted)">离线</span>
        </div>
      </div>
      <div class="theme-toggle" title="切换主题">
        <button class="theme-btn active" data-theme-val="default" title="深空" aria-label="深色主题">
          <svg aria-hidden="true"><use href="#icon-moon"/></svg>
        </button>
        <button class="theme-btn" data-theme-val="light-sky" title="青瓷" aria-label="浅色主题">
          <svg aria-hidden="true"><use href="#icon-sun"/></svg>
        </button>
        <button class="theme-btn" data-theme-val="dark-warm" title="暖阳" aria-label="暖色主题">
          <svg aria-hidden="true"><use href="#icon-warm"/></svg>
        </button>
      </div>
      <div class="alarm-bell" role="button" tabindex="0" aria-label="通知">
        <svg width="18" height="18" aria-hidden="true"><use href="#icon-bell"/></svg>
        <span class="badge-dot"></span>
      </div>
      <div class="user-tag">
        <div class="avatar">${esc(initials)}</div>
        <div>
          <div class="name">${esc(name)}</div>
          <div class="role">${esc(role)}</div>
        </div>
      </div>
    </header>
    <div class="tabbar" id="tabbar" role="tablist" aria-label="已打开的视图"></div>
    <div class="view-container" id="viewContainer"></div>
  </main>
</div>`;

  shellRendered = true;
  wireTabbar();

  // Nav item click handlers：复用已打开的标签；Ctrl/⌘ 点击或中键强制新标签
  document.querySelectorAll('.nav-item[data-view]').forEach(btn => {
    btn.addEventListener('click', (e) => openView(btn.dataset.view, { newTab: e.ctrlKey || e.metaKey }));
    btn.addEventListener('auxclick', (e) => {
      if (e.button !== 1) return;
      e.preventDefault();
      openView(btn.dataset.view, { newTab: true });
    });
  });

  // Panel collapse toggle
  document.getElementById('panelToggle').addEventListener('click', () => {
    const panel = document.getElementById('sidePanel');
    const btn = document.getElementById('panelToggle');
    panel.classList.toggle('collapsed');
    const collapsed = panel.classList.contains('collapsed');
    btn.title = collapsed ? '展开面板' : '折叠面板';
    btn.setAttribute('aria-label', btn.title);
    btn.classList.toggle('collapsed', collapsed);
  });

  // Nav expand/collapse toggle
  const navRail = document.querySelector('.nav-rail');
  const navToggle = document.getElementById('navToggle');
  const setNavExpanded = (expanded) => {
    navRail.classList.toggle('expanded', expanded);
    navToggle.title = expanded ? '收起导航' : '展开导航';
    navToggle.setAttribute('aria-label', navToggle.title);
    navToggle.classList.toggle('expanded', expanded);
    if (expanded) localStorage.setItem('owl-nav-expanded', '1');
    else localStorage.removeItem('owl-nav-expanded');
  };
  navToggle.addEventListener('click', () => setNavExpanded(!navRail.classList.contains('expanded')));
  if (localStorage.getItem('owl-nav-expanded') === '1') setNavExpanded(true);

  // Keyboard shortcuts: alt+1..7 for nav items, Alt+T/W 开关标签，Alt+PageUp/Down 切换
  document.addEventListener('keydown', (e) => {
    if (!e.altKey || e.ctrlKey || e.metaKey) return;
    const key = (e.key || '').toLowerCase();
    if (key === 't') { e.preventDefault(); newTab(); return; }
    if (key === 'w') { e.preventDefault(); if (tabs.activeId) closeTab(tabs.activeId); return; }
    if (key === 'pageup' || key === 'arrowleft') { e.preventDefault(); nextTab(-1); return; }
    if (key === 'pagedown' || key === 'arrowright') { e.preventDefault(); nextTab(1); return; }
    if (e.key >= '1' && e.key <= '7') {
      const idx = parseInt(e.key, 10) - 1;
      if (idx < NAV_ITEMS.length) {
        e.preventDefault();
        openView(NAV_ITEMS[idx].id);
      }
    }
  });

  // Theme toggle
  document.querySelectorAll('.theme-btn').forEach(btn => {
    btn.addEventListener('click', () => setTheme(btn.dataset.themeVal));
  });
  const savedTheme = localStorage.getItem('owl-theme');
  if (savedTheme) setTheme(savedTheme);

  // User tag logout
  const ut = document.querySelector('.user-tag');
  if (ut) {
    ut.addEventListener('click', () => {
      if (confirm('确认退出登录？')) {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        window.location = '/login';
      }
    });
  }

  // Load stats
  loadTopbarStats();
}

function logout() {
  localStorage.removeItem('token');
  localStorage.removeItem('user');
  shellRendered = false;
  tabs.closeAll();   // 标签属于上一个会话：退出即清空，避免换用户后残留
  navigate('/login');
}

function switchView(viewId, pushState) {
  currentViewId = viewId;
  const scope = beginPage(viewId);
  tabs.updateActive({ view: viewId, title: VIEW_TITLES[viewId] || viewId, icon: VIEW_ICONS[viewId] || '' });

  document.querySelectorAll('.nav-item').forEach(n => {
    n.classList.remove('active');
    n.removeAttribute('aria-current');
  });
  const navBtn = document.querySelector(`.nav-item[data-view="${viewId}"]`);
  if (navBtn) {
    navBtn.classList.add('active');
    navBtn.setAttribute('aria-current', 'page');
  }

  shell.setViewTitle(VIEW_TITLES[viewId] || viewId);
  shell.setPanelTitle(PANEL_TITLES[viewId] || '导航');

  // Hide AI's context panel if leaving AI view
  if (viewId !== 'ai') {
    const agentCtx = document.getElementById('agentContextPanel');
    if (agentCtx) agentCtx.remove();
    const sidePanel = document.getElementById('sidePanel');
    if (sidePanel) {
      sidePanel.style.display = '';
      sidePanel.classList.remove('collapsed');
      const toggle = document.getElementById('panelToggle');
      if (toggle) {
        toggle.style.display = '';
        toggle.classList.remove('collapsed');
      }
      const vc = document.querySelector('.view-container');
      if (vc) vc.style.padding = '';
    }
  }

  // Route to the view
  const path = viewId === 'dashboard' ? '/' : '/' + viewId;
  if (pushState !== false) {
    history.pushState(null, '', path);
  }

  const user = getUser();
  if (!user) return;

  switch (viewId) {
    case 'dashboard':
      renderDashboard(render, navigate, user, api, shell, scope);
      break;
    case 'nodes':
      renderNodes(render, navigate, user, api, shell, scope);
      break;
    case 'exec':
      renderExec(render, navigate, user, api, shell, scope);
      break;
    case 'playbooks':
      renderPlaybooks(render, navigate, user, api, shell, scope);
      break;
    case 'files':
      renderFiles(render, navigate, user, api, shell, scope);
      break;
    case 'ai':
      renderAI(render, navigate, user, api, shell, scope);
      break;
    case 'history':
      renderHistory(render, navigate, user, api, shell, scope);
      break;
    case 'alerts':
      renderAlerts(render, navigate, user, api, shell, scope);
      break;
    case 'settings':
      renderSettings(render, navigate, user, api, scope);
      break;
    case 'users':
      renderUsers(render, navigate, user, api, shell, scope);
      break;
    default:
      renderDashboard(render, navigate, user, api, shell, scope);
  }

  updatePanelContent(viewId);
}

let settingsSections = readSettingsSections();

function readSettingsSections() {
  try {
    const s = JSON.parse(localStorage.getItem('owl-settings-sections') || '{}');
    return { ai: s.ai !== false, kv: s.kv !== false, monitor: s.monitor !== false, db: s.db !== false };
  } catch { return { ai: true, kv: true, monitor: true, db: true }; }
}

function dispatchSettingsSections() {
  document.dispatchEvent(new CustomEvent('owl:settings-sections', { detail: { ...settingsSections } }));
}

function updatePanelContent(viewId) {
  const list = document.getElementById('panelList');
  if (!list) return;
  const P = PANEL_TITLES[viewId] || '导航';
  shell.setPanelTitle(P);
  // alerts 页面板（分组过滤）由 renderAlerts 自治渲染，此处不得覆盖
  if (viewId === 'history' || viewId === 'dashboard' || viewId === 'nodes' || viewId === 'exec' || viewId === 'playbooks' || viewId === 'files' || viewId === 'users' || viewId === 'alerts') {
    return;
  }
  if (viewId === 'settings') {
    list.innerHTML = `
      <li class="panel-item" style="cursor:default;color:var(--muted);font-size:12px">显示内容</li>
      <li class="panel-item">
        <label>
          <input type="checkbox" class="group-check settings-sec-check" data-sec="ai" ${settingsSections.ai ? 'checked' : ''}>
          <span class="dot" style="background:${settingsSections.ai ? 'var(--accent)' : 'var(--muted)'}"></span>
          <span class="group-text">AI 供应商</span>
        </label>
      </li>
      <li class="panel-item">
        <label>
          <input type="checkbox" class="group-check settings-sec-check" data-sec="kv" ${settingsSections.kv ? 'checked' : ''}>
          <span class="dot" style="background:${settingsSections.kv ? 'var(--accent)' : 'var(--muted)'}"></span>
          <span class="group-text">KV 配置</span>
        </label>
      </li>
      <li class="panel-item">
        <label>
          <input type="checkbox" class="group-check settings-sec-check" data-sec="monitor" ${settingsSections.monitor ? 'checked' : ''}>
          <span class="dot" style="background:${settingsSections.monitor ? 'var(--accent)' : 'var(--muted)'}"></span>
          <span class="group-text">监控告警</span>
        </label>
      </li>
      <li class="panel-item">
        <label>
          <input type="checkbox" class="group-check settings-sec-check" data-sec="db" ${settingsSections.db ? 'checked' : ''}>
          <span class="dot" style="background:${settingsSections.db ? 'var(--accent)' : 'var(--muted)'}"></span>
          <span class="group-text">数据库统计</span>
        </label>
      </li>`;
    document.querySelectorAll('.settings-sec-check').forEach(cb => {
      cb.addEventListener('change', () => {
        settingsSections[cb.dataset.sec] = cb.checked;
        localStorage.setItem('owl-settings-sections', JSON.stringify(settingsSections));
        dispatchSettingsSections();
      });
    });
    return;
  }
  list.innerHTML = '<li class="panel-item" style="cursor:default;color:var(--muted);font-size:12px">加载中…</li>';
}

function renderPlaceholderView(viewId, title) {
  const container = document.querySelector('.view-container');
  if (!container) return;
  container.innerHTML = `
<div class="view" style="display:flex;flex-direction:column;flex:1;gap:20px;align-items:center;justify-content:center;padding:60px 24px">
  <div class="view-empty">
    <div class="empty-icon" style="opacity:0.2;font-size:48px">
      <svg width="64" height="64" aria-hidden="true"><use href="#icon-${viewId === 'exec' ? 'terminal' : viewId === 'files' ? 'upload' : viewId === 'ai' ? 'brain' : viewId === 'history' ? 'clock' : 'dashboard'}"/></svg>
    </div>
    <div class="empty-title">${esc(title)}</div>
    <div class="empty-desc">此视图将在后续阶段实现。<br>基础 shell 架构（导航栏、面板、顶部栏、主题切换）已就绪。</div>
  </div>
</div>`;
}

function navigate(path) {
  history.pushState(null, '', path);
  router();
}

function getUser() {
  const raw = localStorage.getItem('user');
  return raw ? JSON.parse(raw) : null;
}

function requireAuth() {
  const u = getUser();
  if (!u || !localStorage.getItem('token')) {
    navigate('/login');
    return null;
  }
  return u;
}

function loadTopbarStats() {
  if (!api || !api.nodeStats) return;
  api.nodeStats().then(res => {
    const online = document.getElementById('statOnline');
    const offline = document.getElementById('statOffline');
    if (online) online.textContent = res.online || 0;
    if (offline) offline.textContent = res.offline || 0;
  }).catch(() => {});
}

function setTheme(theme) {
  if (theme === 'default') {
    document.documentElement.removeAttribute('data-theme');
  } else {
    document.documentElement.setAttribute('data-theme', theme);
  }
  localStorage.setItem('owl-theme', theme);
  document.querySelectorAll('.theme-btn').forEach(b => {
    b.classList.toggle('active', b.dataset.themeVal === theme);
  });
}

function router() {
  const path = location.pathname.replace(/\/+$/, '') || '/';

  if (path === '/login') {
    shellRendered = false;
    if (tabs.tabs.length) tabs.closeAll();   // 会话失效/退出：不把上个用户的标签带进登录页
    beginPage('login');
    renderLogin(render, navigate);
    return;
  }

  const u = requireAuth();
  if (!u) return;

  // Ensure shell is rendered for any authenticated route
  renderShell();
  bootTabs();               // 对齐持久化标签集合与当前 URL（URL 优先）
  syncActiveTabRoute();     // 当前 URL 记到激活标签上

  const nodeMatch = path.match(/^\/nodes\/(.+)/);
  const taskMatch = path.match(/^\/tasks\/(.+)/);
  const termMatch = path.match(/^\/terminal\/(.+)/);
  const sftpMatch = path.match(/^\/sftp\/(.+)/);

  if (path === '/') {
    switchView('dashboard', false);
  } else if (path === '/alerts') {
    switchView('alerts', false);
  } else if (path === '/nodes') {
    switchView('nodes', false);
  } else if (termMatch) {
    const termNodeId = decodeURIComponent(termMatch[1]);
    setViewMetadata('终端', '节点管理');
    const scope = beginPage('terminal');
    import('./pages/terminal.js').then(m => {
      m.renderTerminal(render, navigate, u, api, termNodeId, scope);
    });
  } else if (sftpMatch) {
    const sftpNodeId = decodeURIComponent(sftpMatch[1]);
    setViewMetadata('文件管理', '节点管理');
    const scope = beginPage('sftp');
    import('./pages/sftp.js').then(m => {
      m.renderSftp(render, navigate, u, api, sftpNodeId, scope);
    });
  } else if (nodeMatch) {
    setViewMetadata('节点详情', '节点管理');
    renderNodeDetail(render, navigate, u, api, decodeURIComponent(nodeMatch[1]), beginPage('node-detail'));
  } else if (path === '/exec') {
    switchView('exec', false);
  } else if (path === '/tasks' || path === '/tasks/') {
    switchView('history', false);
  } else if (taskMatch) {
    const taskId = decodeURIComponent(taskMatch[1]);
    setViewMetadata('任务详情', '任务历史');
    const scope = beginPage('task-detail');
    import('./pages/task_detail.js').then(m => {
      const cleanup = m.renderTaskDetail(render, navigate, u, api, taskId, scope);
      if (typeof cleanup === 'function') currentCleanup = cleanup;
    });
  } else if (path === '/playbooks') {
    switchView('playbooks', false);
  } else if (path === '/files') {
    switchView('files', false);
  } else if (path === '/ai') {
    switchView('ai', false);
  } else if (path === '/history') {
    switchView('history', false);
  } else if (path === '/settings') {
    switchView('settings', false);
  } else if (path === '/users') {
    switchView('users', false);
  } else {
    history.replaceState(null, '', '/');
    switchView('dashboard', false);
  }
}

function setViewMetadata(title, breadcrumb) {
  document.getElementById('viewTitle').textContent = title;
  document.getElementById('breadcrumbCurrent').textContent = breadcrumb || title;
  tabs.updateActive({ title });
  // Activate closest nav item
  document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
  const route = title === '节点详情' ? 'nodes' : title === '任务详情' ? 'history' : '';
  if (route) {
    const btn = document.querySelector(`.nav-item[data-view="${route}"]`);
    if (btn) { btn.classList.add('active'); btn.setAttribute('aria-current', 'page'); }
  }
}

window.addEventListener('popstate', router);
document.addEventListener('DOMContentLoaded', router);
