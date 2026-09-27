// 标签模型（M1：单挂载、重建式标签）
//
// 语义：一次只挂载一个标签的页面。切换标签 = 把当前页的状态快照写回它的标签，
// 释放该页资源（见 pagescope.js），再按目标标签的路由重新挂载页面。
// 因此标签要记住三样东西：路由（URL 唯一来源）、页面状态快照（内存）、滚动位置。
//
// 持久化：只把「标签集合 + 激活项 + 路由/标题」写进 localStorage；状态快照留在内存，
// 刷新后按路由重建（避免把上百个节点的列表塞进 5MB 配额里）。
//
// 本模块只管数据，不碰 DOM（标签栏渲染在 app.js）。

const STORAGE_KEY = 'owl-tabs';
const STORAGE_VERSION = 1;
const MAX_TABS = 8;

export function createTabStore() {
  let tabs = [];
  let activeId = null;
  let seq = 0;
  let loaded = false;
  const listeners = [];

  function notify() {
    listeners.forEach(fn => { try { fn(); } catch (e) { console.warn('tab listener failed:', e); } });
  }

  function save() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        v: STORAGE_VERSION,
        activeId,
        seq,
        tabs: tabs.map(t => ({ id: t.id, view: t.view, route: t.route, title: t.title, icon: t.icon })),
      }));
    } catch { /* 配额/隐私模式：不阻塞标签操作 */ }
  }

  // 读取持久化的标签集合（幂等；URL 的权威性由调用方保证）
  function restore() {
    if (loaded) return;
    loaded = true;
    try {
      const raw = JSON.parse(localStorage.getItem(STORAGE_KEY) || 'null');
      if (!raw || raw.v !== STORAGE_VERSION || !Array.isArray(raw.tabs)) return;
      tabs = raw.tabs
        .filter(t => t && t.id && t.view)
        .slice(0, MAX_TABS)
        .map(t => ({
          id: t.id, view: t.view, route: t.route || '/', title: t.title || t.view,
          icon: t.icon || '', snapshots: {}, scroll: {}, keepAlive: false,
        }));
      seq = raw.seq || tabs.length;
      activeId = tabs.some(t => t.id === raw.activeId) ? raw.activeId : (tabs[0] ? tabs[0].id : null);
    } catch { tabs = []; activeId = null; }
  }

  const api = {
    get tabs() { return tabs; },
    get activeId() { return activeId; },
    get limit() { return MAX_TABS; },

    active() { return tabs.find(t => t.id === activeId) || null; },
    get(id) { return tabs.find(t => t.id === id) || null; },
    byView(view) { return tabs.filter(t => t.view === view); },

    create(view, route, opts = {}) {
      const tab = {
        id: 'tab-' + (++seq),
        view,
        route: route || '/',
        title: opts.title || view,
        icon: opts.icon || '',
        snapshots: {},   // 页面状态快照：{ [pageName]: state }（内存）
        scroll: {},      // 滚动位置：{ [pageName]: scrollTop }（内存）
        keepAlive: false,
      };
      tabs.push(tab);
      save();
      notify();
      return tab;
    },

    update(id, patch) {
      const tab = api.get(id);
      if (!tab) return null;
      Object.assign(tab, patch);
      save();
      notify();
      return tab;
    },

    updateActive(patch) { return activeId ? api.update(activeId, patch) : null; },

    // 快照与滚动位置是内存态：不持久化，也不必触发标签栏重绘
    setSnapshot(id, pageName, snap) {
      const tab = api.get(id);
      if (!tab) return;
      if (snap) tab.snapshots[pageName] = snap;
      else delete tab.snapshots[pageName];
    },

    setScroll(id, pageName, top) {
      const tab = api.get(id);
      if (tab) tab.scroll[pageName] = top || 0;
    },

    activate(id) {
      if (!api.get(id) || id === activeId) return;
      activeId = id;
      save();
      notify();
    },

    // 关闭标签，返回 { wasActive, nextActiveId }
    close(id) {
      const idx = tabs.findIndex(t => t.id === id);
      if (idx < 0) return { wasActive: false, nextActiveId: activeId };
      const wasActive = id === activeId;
      tabs.splice(idx, 1);
      let nextActiveId = activeId;
      if (wasActive) {
        const neighbor = tabs[idx] || tabs[idx - 1] || null;   // 右邻优先，其次左邻
        nextActiveId = neighbor ? neighbor.id : null;
        activeId = nextActiveId;
      }
      save();
      notify();
      return { wasActive, nextActiveId };
    },

    closeOthers(id) {
      tabs = tabs.filter(t => t.id === id);
      if (!api.get(activeId)) activeId = id;
      save();
      notify();
    },

    closeRight(id) {
      const idx = tabs.findIndex(t => t.id === id);
      if (idx < 0) return;
      tabs = tabs.slice(0, idx + 1);
      if (!api.get(activeId)) activeId = id;
      save();
      notify();
    },

    closeAll() {
      tabs = [];
      activeId = null;
      save();
      notify();
    },

    move(id, toIndex) {
      const from = tabs.findIndex(t => t.id === id);
      if (from < 0) return;
      const to = Math.max(0, Math.min(tabs.length - 1, toIndex));
      if (from === to) return;
      const [tab] = tabs.splice(from, 1);
      tabs.splice(to, 0, tab);
      save();
      notify();
    },

    // 关闭激活标签后的善后：一个都不剩时补一个默认标签
    ensureOne(fallbackView, fallbackRoute) {
      if (tabs.length) return null;
      const tab = api.create(fallbackView || 'dashboard', fallbackRoute || '/');
      api.activate(tab.id);
      return tab;
    },

    subscribe(fn) { if (typeof fn === 'function') listeners.push(fn); },
    save,
    restore,
  };

  return api;
}

export const TAB_LIMIT = MAX_TABS;
