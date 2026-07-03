import { renderLogin } from './pages/login.js';
import { renderDashboard } from './pages/dashboard.js';
import { renderNodeDetail } from './pages/node.js';
import { renderTasks } from './pages/tasks.js';
import { renderSettings } from './pages/settings.js';
import { renderUsers } from './pages/users.js';
import { renderPlaybooks } from './pages/playbooks.js';

let currentCleanup = null;

function render(html, afterRender) {
  const app = document.getElementById('app');
  app.innerHTML = html;
  if (currentCleanup) currentCleanup();
  currentCleanup = null;
  if (afterRender) {
    const cleanup = afterRender();
    if (typeof cleanup === 'function') currentCleanup = cleanup;
  }
}

export function navigate(path) {
  history.pushState(null, '', path);
  router();
}

function user() {
  const raw = localStorage.getItem('user');
  return raw ? JSON.parse(raw) : null;
}

function requireAuth() {
  const u = user();
  if (!u || !localStorage.getItem('token')) {
    navigate('/login');
    return null;
  }
  return u;
}

function router() {
  const path = location.pathname.replace(/\/+$/, '') || '/';

  // Login page is public
  if (path === '/login') {
    renderLogin(render, navigate);
    return;
  }

  const u = requireAuth();
  if (!u) return;

  const nodeMatch = path.match(/^\/nodes\/(.+)/);
  const taskMatch = path.match(/^\/tasks\/(.+)/);

  if (path === '/' || path === '/nodes') {
    renderDashboard(render, navigate, u, api);
  } else if (nodeMatch) {
    renderNodeDetail(render, navigate, u, api, decodeURIComponent(nodeMatch[1]));
  } else if (path === '/tasks') {
    renderTasks(render, navigate, u, api);
  } else if (taskMatch) {
    import('./pages/task_detail.js').then(m => { const cleanup = m.renderTaskDetail(render, navigate, u, api, decodeURIComponent(taskMatch[1])); if (typeof cleanup === 'function') currentCleanup = cleanup; });
  } else if (path === '/playbooks') {
    renderPlaybooks(render, navigate, u, api);
  } else if (path === '/users') {
    renderUsers(render, navigate, u, api);
  } else if (path === '/settings') {
    renderSettings(render, navigate, u, api);
  } else {
    navigate('/');
  }
}

window.addEventListener('popstate', router);
document.addEventListener('DOMContentLoaded', router);
