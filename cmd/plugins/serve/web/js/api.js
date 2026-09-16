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

async function downloadFile(path, fallbackName) {
  const t = token();
  const res = await fetch(`${API_BASE}${path}`, { headers: { 'Authorization': `Bearer ${t}` } });
  if (res.status === 401) {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }
  if (!res.ok) throw new Error('下载失败');
  const disposition = res.headers.get('Content-Disposition') || '';
  const match = disposition.match(/filename="?([^";]+)"?/);
  const filename = (match && match[1]) || fallbackName;
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

export const api = {
  login: (username, password) =>
    request('POST', '/login', { username, password }),

  me: () =>
    request('GET', '/me'),

  nodes: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined || v === null || v === '') continue;
      if (Array.isArray(v)) v.forEach(x => { if (x !== '' && x != null) q.append(k, x); });
      else q.set(k, v);
    }
    return request('GET', `/nodes?${q}`);
  },

  nodeStats: () =>
    request('GET', '/nodes/stats'),

  node: (id) =>
    request('GET', `/nodes/${encodeURIComponent(id)}`),

  filters: () =>
    request('GET', '/nodes/filters'),

  shortcuts: () =>
    request('GET', '/shortcuts'),

  createShortcut: (data) =>
    request('POST', '/shortcuts', data),

  updateShortcut: (id, data) =>
    request('PUT', `/shortcuts/${encodeURIComponent(id)}`, data),

  deleteShortcut: (id) =>
    request('DELETE', `/shortcuts/${encodeURIComponent(id)}`),

  reorderShortcuts: (orderedIds) =>
    request('PUT', '/shortcuts/reorder', { ordered_ids: orderedIds }),

  searchNodes: (q) =>
    request('GET', `/nodes/search?q=${encodeURIComponent(q)}`),

  createNode: (data) =>
    request('POST', '/nodes', data),

  updateNode: (id, data) =>
    request('PUT', `/nodes/${encodeURIComponent(id)}`, data),

  deleteNode: (id) =>
    request('DELETE', `/nodes/${encodeURIComponent(id)}`),

  batchGroup: (nodeIds, { add, remove }) =>
    request('POST', '/nodes/batch/groups', { node_ids: nodeIds, add: add || [], remove: remove || [] }),

  exportNodes: async (params) => {
    const t = token();
    const res = await fetch(`${API_BASE}/nodes/export`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${t}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    if (res.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/login';
      throw new Error('Unauthorized');
    }
    if (!res.ok) throw new Error('Export failed');
    const disposition = res.headers.get('Content-Disposition') || '';
    const match = disposition.match(/filename=(.+)/);
    const filename = match ? match[1] : 'nodes.yaml';
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  },

  importNodes: async (formData) => {
    const t = token();
    const res = await fetch(`${API_BASE}/nodes/import`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${t}` },
      body: formData,
    });
    if (res.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/login';
      throw new Error('Unauthorized');
    }
    if (!res.ok) {
      const err = await res.json().catch(() => ({ message: res.statusText }));
      throw new Error(err.message || 'Import failed');
    }
    return res.json();
  },

  pingNodes: (nodeIds) =>
    request('POST', '/nodes/ping', { node_ids: nodeIds }),

  checkNodes: (nodeIds) =>
    request('POST', '/nodes/check', { node_ids: nodeIds }),

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

  users: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined || v === null || v === '') continue;
      q.set(k, v);
    }
    return request('GET', `/users?${q}`);
  },

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

  playbookGet: (id) =>
    request('GET', `/playbooks/${encodeURIComponent(id)}`),

  playbookFile: (id) =>
    request('GET', `/playbooks/${encodeURIComponent(id)}/file`),

  playbookEdit: (id) =>
    request('GET', `/playbooks/${encodeURIComponent(id)}/edit`),

  playbookDownload: async (id) => {
    const t = token();
    const res = await fetch(`${API_BASE}/playbooks/${encodeURIComponent(id)}/download`, {
      headers: { 'Authorization': `Bearer ${t}` },
    });
    if (!res.ok) throw new Error('Download failed');
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const cd = res.headers.get('Content-Disposition') || '';
    const m = cd.match(/filename="?([^";]+)"?/);
    const a = document.createElement('a');
    a.href = url;
    a.download = m ? m[1] : 'playbook.yaml';
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  },

  playbookUpload: async (file) => {
    const t = token();
    const formData = new FormData();
    formData.append('file', file);
    const res = await fetch(`${API_BASE}/playbooks/upload`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${t}` },
      body: formData,
    });
    if (res.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/login';
      throw new Error('Unauthorized');
    }
    if (!res.ok) {
      const err = await res.json().catch(() => ({ message: res.statusText }));
      throw new Error(err.message || 'Upload failed');
    }
    return res.json();
  },

  playbookSettingsPath: () =>
    request('GET', '/playbook/settings/path'),

  createPlaybookTemplate: (data) =>
    request('POST', '/playbook/template', data),

  getSessionKey: () => request('GET', '/ai/session-key'),
  aiPermissions: () => request('GET', '/ai/permissions'),
  aiChat: (message, sessionId, encryptedApiKey, provider, model, baseUrl, apiType) => request('POST', '/ai/chat', { message, session_id: sessionId, encrypted_api_key: encryptedApiKey, provider, model, base_url: baseUrl, api_type: apiType }),
  getAiContext: () => request('GET', '/ai/context'),
  aiAudit: (record) => request('POST', '/ai/audit', record),
  aiModels: (sessionId, encryptedApiKey, baseUrl, apiType) => request('POST', '/ai/models', { session_id: sessionId, encrypted_api_key: encryptedApiKey, base_url: baseUrl, api_type: apiType }),
  aiTest: (sessionId, encryptedApiKey, baseUrl, apiType, model) => request('POST', '/ai/test', { session_id: sessionId, encrypted_api_key: encryptedApiKey, base_url: baseUrl, api_type: apiType, model }),

  staging: {
    files: () =>
      request('GET', '/staging/files'),

    disk: () =>
      request('GET', '/staging/disk'),

    upload: async (file) => {
      const t = token();
      const formData = new FormData();
      formData.append('file', file);
      const res = await fetch(`${API_BASE}/staging/upload`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${t}` },
        body: formData,
      });
      if (res.status === 401) {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        window.location.href = '/login';
        throw new Error('Unauthorized');
      }
      if (!res.ok) {
        const err = await res.json().catch(() => ({ message: res.statusText }));
        throw new Error(err.message || 'Upload failed');
      }
      return res.json();
    },

    delete: (name) =>
      request('DELETE', `/staging/${encodeURIComponent(name)}`),
  },

  transfer: (data) =>
    request('POST', '/transfer', data),

  transfers: () =>
    request('GET', '/transfers'),

  transferRecords: () =>
    request('GET', '/transfer/records'),

  transferRecord: (id) =>
    request('GET', `/transfer/records/${encodeURIComponent(id)}`),

  transferRerun: (id) =>
    request('POST', `/transfer/records/${encodeURIComponent(id)}/rerun`),

  historyList: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') q.set(k, v);
    return request('GET', `/history?${q}`);
  },

  historyStats: () =>
    request('GET', '/history/stats'),

  historyGet: (taskId) =>
    request('GET', `/history/detail/${encodeURIComponent(taskId)}`),

  historyClean: (days) =>
    request('DELETE', `/history?days=${encodeURIComponent(days)}`),

  executionLogs: (opId) =>
    request('GET', `/executions/${encodeURIComponent(opId)}/logs`),

  executionLogDownload: (opId, nodeId) =>
    downloadFile(`/executions/${encodeURIComponent(opId)}/logs/${encodeURIComponent(nodeId)}`, `${nodeId}.log`),

  executionLogArchive: (opId) =>
    downloadFile(`/executions/${encodeURIComponent(opId)}/logs/archive`, `executions-${opId}.zip`),

  historyExport: async (params = {}, format = 'json') => {
    const t = token();
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') q.set(k, v);
    q.set('format', format);
    const res = await fetch(`${API_BASE}/history/export?${q}`, {
      method: 'GET',
      headers: { 'Authorization': `Bearer ${t}` },
    });
    if (res.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/login';
      throw new Error('Unauthorized');
    }
    if (!res.ok) throw new Error('Export failed');
    const disposition = res.headers.get('Content-Disposition') || '';
    const match = disposition.match(/filename=(.+)/);
    const filename = match ? match[1] : `history.${format === 'yaml' ? 'yaml' : 'json'}`;
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  },

  // 取 WebSocket 建连票据（一次性，60 秒有效）
  wsTicket: () => request('POST', '/ws/ticket'),

  // 建连改用一次性票据：长期 JWT 不再出现在 URL 中（避免进反代日志/浏览器历史）。
  // 保持同步签名返回句柄，票据异步获取，未取到则保持断开。
  connectWebSocket(onMessage) {
    let ws = null;
    let closed = false;
    let reconnectTimer = null;

    const open = () => {
      if (closed) return;
      api.wsTicket().then((res) => {
        if (closed) return;
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws?ticket=${encodeURIComponent(res.ticket)}`);
        ws.onmessage = (event) => {
          try {
            const msg = JSON.parse(event.data);
            if (onMessage) onMessage(msg);
          } catch {}
        };
        ws.onclose = () => {
          if (closed) return;
          reconnectTimer = setTimeout(open, 3000);
        };
        ws.onerror = () => { if (ws) ws.close(); };
      }).catch(() => { /* 取票据失败（未登录等）：保持断开 */ });
    };

    open();

    return {
      close: () => {
        closed = true;
        if (reconnectTimer) clearTimeout(reconnectTimer);
        if (ws) ws.close();
      }
    };
  },

  // ---------- 监控告警 ----------
  alerts: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined || v === null || v === '') continue;
      q.set(k, v);
    }
    return request('GET', `/alerts?${q}`);
  },

  alert: (id) =>
    request('GET', `/alerts/${encodeURIComponent(id)}`),

  alertAck: (id) =>
    request('POST', `/alerts/${encodeURIComponent(id)}/ack`, {}),

  alertResolve: (id) =>
    request('POST', `/alerts/${encodeURIComponent(id)}/resolve`, {}),

  alertTypes: () =>
    request('GET', '/alert-types'),

  createAlertType: (data) =>
    request('POST', '/alert-types', data),

  deleteAlertType: (id) =>
    request('DELETE', `/alert-types/${encodeURIComponent(id)}`),

  testAlertType: (id, data) =>
    request('POST', `/alert-types/${encodeURIComponent(id)}/test`, data || {}),

  updateAlertType: (id, data) =>
    request('PUT', `/alert-types/${encodeURIComponent(id)}`, data),

  remedies: (alertTypeId) => {
    const q = alertTypeId ? `?alert_type_id=${encodeURIComponent(alertTypeId)}` : '';
    return request('GET', `/remedies${q}`);
  },

  createRemedy: (data) =>
    request('POST', '/remedies', data),

  updateRemedy: (id, data) =>
    request('PUT', `/remedies/${encodeURIComponent(id)}`, data),

  deleteRemedy: (id) =>
    request('DELETE', `/remedies/${encodeURIComponent(id)}`),

  notifyChannels: () =>
    request('GET', '/notify-channels'),

  createNotifyChannel: (data) =>
    request('POST', '/notify-channels', data),

  updateNotifyChannel: (id, data) =>
    request('PUT', `/notify-channels/${encodeURIComponent(id)}`, data),

  deleteNotifyChannel: (id) =>
    request('DELETE', `/notify-channels/${encodeURIComponent(id)}`),

  testNotifyChannel: (id) =>
    request('POST', `/notify-channels/${encodeURIComponent(id)}/test`, {}),

  monitorSilence: () =>
    request('GET', '/monitor/silence'),

  setMonitorSilence: (until) =>
    request('PUT', '/monitor/silence', { silence_until: until }),

  metrics: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined || v === null || v === '') continue;
      q.set(k, v);
    }
    return request('GET', `/metrics?${q}`);
  },

  // ---------- 处置计划 ----------
  createRemedyPlan: (alertId, data) =>
    request('POST', `/alerts/${encodeURIComponent(alertId)}/plans`, data),

  remedyPlan: (id) =>
    request('GET', `/plans/${encodeURIComponent(id)}`),

  remedyPlans: (alertId) =>
    request('GET', `/alerts/${encodeURIComponent(alertId)}/plans`),

  stopRemedyPlan: (id) =>
    request('POST', `/plans/${encodeURIComponent(id)}/stop`, {}),

  approveRemedyPlan: (id) =>
    request('POST', `/plans/${encodeURIComponent(id)}/approve`, {}),

  rejectRemedyPlan: (id) =>
    request('POST', `/plans/${encodeURIComponent(id)}/reject`, {}),

  // ---------- 告警专属指令绑定（按告警 ID 指定 playbook/脚本） ----------
  alertBindings: (alertId) =>
    request('GET', `/alerts/${encodeURIComponent(alertId)}/bindings`),

  createAlertBinding: (alertId, data) =>
    request('POST', `/alerts/${encodeURIComponent(alertId)}/bindings`, data),

  updateAlertBinding: (id, data) =>
    request('PUT', `/alert-bindings/${encodeURIComponent(id)}`, data),

  deleteAlertBinding: (id) =>
    request('DELETE', `/alert-bindings/${encodeURIComponent(id)}`),

  runAlertBindings: (alertId, data) =>
    request('POST', `/alerts/${encodeURIComponent(alertId)}/bindings/run`, data),

  alertBindingRuns: (alertId) =>
    request('GET', `/alerts/${encodeURIComponent(alertId)}/binding-runs`),
};
