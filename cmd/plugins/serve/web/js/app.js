import { api } from './api.js';
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

let currentCleanup = null;
let shellRendered = false;
let currentViewId = null;

const VIEW_TITLES = {
  dashboard: '仪表盘', nodes: '节点管理', exec: '命令执行',
  playbooks: '剧本管理', files: '文件传输', ai: 'AI 助手',
  history: '任务历史', settings: '系统设置', users: '用户管理'
};

const PANEL_TITLES = {
  dashboard: '概览', nodes: '节点分组', exec: '节点选择',
  playbooks: '剧本分类', files: '节点选择', ai: '对话上下文',
  history: '过滤条件', settings: '系统配置', users: '用户角色'
};

const NAV_ITEMS = [
  { id: 'dashboard', icon: 'dashboard', label: '仪表盘' },
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
    <div class="logo" aria-label="OWL Console 首页"><span>O</span></div>
    ${NAV_ITEMS.map(item => `
      <button class="nav-item" data-view="${item.id}" title="${item.label}" aria-label="${item.label}">
        <svg aria-hidden="true"><use href="#icon-${item.icon}"/></svg>
      </button>
    `).join('')}
    <div class="nav-spacer"></div>
    ${NAV_BOTTOM.map(item => `
      <button class="nav-item" data-view="${item.id}" title="${item.label}" aria-label="${item.label}">
        <svg aria-hidden="true"><use href="#icon-${item.icon}"/></svg>
      </button>
    `).join('')}
    <div class="nav-avatar" title="个人设置">${esc(initials)}</div>
  </nav>
  <aside class="panel" id="sidePanel">
    <div class="panel-header" id="panelTitle">
      <span>概览</span>
    </div>
    <ul class="panel-list" id="panelList"></ul>
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
        <button class="theme-btn" data-theme-val="light-sky" title="薄荷绿" aria-label="浅色主题">
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
    <div class="view-container" id="viewContainer"></div>
  </main>
</div>`;

  shellRendered = true;

  // Nav item click handlers
  document.querySelectorAll('.nav-item[data-view]').forEach(btn => {
    btn.addEventListener('click', () => switchView(btn.dataset.view));
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

  // Keyboard shortcuts: alt+1..7 for nav items
  document.addEventListener('keydown', (e) => {
    if (e.altKey && !e.ctrlKey && !e.metaKey && e.key >= '1' && e.key <= '7') {
      const idx = parseInt(e.key) - 1;
      if (idx < NAV_ITEMS.length) {
        e.preventDefault();
        switchView(NAV_ITEMS[idx].id);
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
  navigate('/login');
}

function switchView(viewId, pushState) {
  const currentNav = document.querySelector('.nav-item.active');
  if (currentNav && currentNav.dataset.view === viewId) return;
  currentViewId = viewId;

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
      renderDashboard(render, navigate, user, api, shell);
      break;
    case 'nodes':
      renderNodes(render, navigate, user, api, shell);
      break;
    case 'exec':
      renderExec(render, navigate, user, api, shell);
      break;
    case 'playbooks':
      renderPlaybooks(render, navigate, user, api, shell);
      break;
    case 'files':
      renderFiles(render, navigate, user, api, shell);
      break;
    case 'ai':
      renderAI(render, navigate, user, api, shell);
      break;
    case 'history':
      renderHistory(render, navigate, user, api, shell);
      break;
    case 'settings':
      renderSettings(render, navigate, user, api);
      break;
    case 'users':
      renderUsers(render, navigate, user, api);
      break;
    default:
      renderDashboard(render, navigate, user, api);
  }

  updatePanelContent(viewId);
}

function updatePanelContent(viewId) {
  const list = document.getElementById('panelList');
  if (!list) return;
  const P = PANEL_TITLES[viewId] || '导航';
  shell.setPanelTitle(P);
  if (viewId === 'history' || viewId === 'dashboard' || viewId === 'nodes' || viewId === 'exec' || viewId === 'playbooks' || viewId === 'files') {
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
  if (!api || !api.nodes) return;
  api.nodes({ page: 1, page_size: 1 }).then(res => {
    // Minimal: just show counts from first page meta if available
  }).catch(() => {});
  // Phase 2+: fetch real stats from API
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
    renderLogin(render, navigate);
    return;
  }

  const u = requireAuth();
  if (!u) return;

  // Ensure shell is rendered for any authenticated route
  renderShell();

  const nodeMatch = path.match(/^\/nodes\/(.+)/);
  const taskMatch = path.match(/^\/tasks\/(.+)/);
  const termMatch = path.match(/^\/terminal\/(.+)/);

  if (path === '/') {
    switchView('dashboard', false);
  } else if (path === '/nodes') {
    switchView('nodes', false);
  } else if (termMatch) {
    const termNodeId = decodeURIComponent(termMatch[1]);
    setViewMetadata('终端', '节点管理');
    import('./pages/terminal.js').then(m => {
      m.renderTerminal(render, navigate, u, api, termNodeId);
    });
  } else if (nodeMatch) {
    setViewMetadata('节点详情', '节点管理');
    renderNodeDetail(render, navigate, u, api, decodeURIComponent(nodeMatch[1]));
  } else if (path === '/exec') {
    switchView('exec', false);
  } else if (path === '/tasks' || path === '/tasks/') {
    switchView('history', false);
  } else if (taskMatch) {
    const taskId = decodeURIComponent(taskMatch[1]);
    setViewMetadata('任务详情', '任务历史');
    import('./pages/task_detail.js').then(m => {
      const cleanup = m.renderTaskDetail(render, navigate, u, api, taskId);
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
