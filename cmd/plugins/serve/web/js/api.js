const API_BASE = '/api/v1';

function token() {
  return localStorage.getItem('token');
}

function authHeaders() {
  const t = token();
  return t ? { 'Authorization': `Bearer ${t}`, 'Content-Type': 'application/json' } : { 'Content-Type': 'application/json' };
}

async function request(method, path, body) {
  const opts = { method, headers: authHeaders() };
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(`${API_BASE}${path}`, opts);
  if (res.status === 401) {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({ message: res.statusText }));
    throw new Error(err.message || 'Request failed');
  }
  return res.json();
}

const api = {
  login: (username, password) =>
    request('POST', '/login', { username, password }),

  me: () =>
    request('GET', '/me'),

  nodes: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
    return request('GET', `/nodes?${q}`);
  },

  node: (id) =>
    request('GET', `/nodes/${encodeURIComponent(id)}`),

  filters: () =>
    request('GET', '/nodes/filters'),

  searchNodes: (q) =>
    request('GET', `/nodes/search?q=${encodeURIComponent(q)}`),

  createNode: (data) =>
    request('POST', '/nodes', data),

  updateNode: (id, data) =>
    request('PUT', `/nodes/${encodeURIComponent(id)}`, data),

  deleteNode: (id) =>
    request('DELETE', `/nodes/${encodeURIComponent(id)}`),

  exec: (nodeId, command, force) =>
    request('POST', '/exec', { node_id: nodeId, command, ...(force ? { force: 'true' } : {}) }),

  execAdvanced: (data) =>
    request('POST', '/exec', data),

  tasks: (params) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params || {})) if (v) q.set(k, v);
    return request('GET', `/tasks?${q}`);
  },

  task: (id) =>
    request('GET', `/tasks/${encodeURIComponent(id)}`),

  cancelTask: (id) =>
    request('DELETE', `/tasks/${encodeURIComponent(id)}`),

  settings: () =>
    request('GET', '/settings'),

  setting: (key) =>
    request('GET', `/settings/${encodeURIComponent(key)}`),

  updateSetting: (key, value) =>
    request('PUT', `/settings/${encodeURIComponent(key)}`, { value }),

  users: () =>
    request('GET', '/users'),

  createUser: (data) =>
    request('POST', '/users', data),

  updateUser: (id, data) =>
    request('PUT', `/users/${encodeURIComponent(id)}`, data),

  deleteUser: (id) =>
    request('DELETE', `/users/${encodeURIComponent(id)}`),

  playbooks: () =>
    request('GET', '/playbooks'),

  refreshPlaybooks: (path) =>
    request('POST', '/playbook/refresh', { path }),

  runPlaybook: (name, data) =>
    request('POST', `/playbooks/${encodeURIComponent(name)}/run`, data),

  playbookRuns: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
    return request('GET', `/playbook/runs?${q}`);
  },

  playbookRun: (id) =>
    request('GET', `/playbook/runs/${encodeURIComponent(id)}`),

  cancelPlaybookRun: (id) =>
    request('DELETE', `/playbook/runs/${encodeURIComponent(id)}`),

  playbookSettingsPath: () =>
    request('GET', '/playbook/settings/path'),

  connectWebSocket(onMessage) {
    const token = localStorage.getItem('token');
    if (!token) return null;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${window.location.host}/api/v1/ws?token=${token}`;
    let ws = new WebSocket(url);
    let reconnectTimer = null;

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (onMessage) onMessage(msg);
      } catch {}
    };

    ws.onclose = () => {
      reconnectTimer = setTimeout(() => {
        connectWebSocket(onMessage);
      }, 3000);
    };

    ws.onerror = () => { ws.close(); };

    return {
      close: () => {
        if (reconnectTimer) clearTimeout(reconnectTimer);
        if (ws) ws.close();
      }
    };
  },
};
