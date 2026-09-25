// 页面资源作用域：页面把「切走时必须回收的东西」注册进来，导航时统一释放。
//
// 为什么需要：这些资源原先靠各页自己写 cleanup 返回给 app.js，漏一处就永久残留
// （历史问题：files 页 5s 轮询的 stopAutoRefresh 全仓无调用点、nodes/settings 的
// document keydown 每次挂载加一条、alerts 递归轮询无取消句柄、body 级浮层跨页残留）。
// 现在统一走 scope：注册即被跟踪，dispose() 逆序释放，为标签页保活（多实例）打底。
//
// 用法（页面签名最后多接一个 scope 参数）：
//   export function renderX(render, navigate, user, api, shell, scope) {
//     scope.resources.setInterval(poll, 5000);
//     scope.resources.on(document, 'keydown', onKey);
//     scope.resources.ws(api.connectWebSocket(onMsg));
//     scope.resources.overlay(el);           // 挂到 body 并登记，切页时移除
//     scope.resources.onDispose(() => ac.abort());
//   }

export function createPageScope(name = 'page', options = {}) {
  let disposalQueue = [];
  let disposed = false;
  let snapshot = options.snapshot && typeof options.snapshot === 'object' ? options.snapshot : null;
  let stateGetter = null;
  const tabId = options.tabId || null;

  // 注册一个释放动作；返回「撤销注册」的函数（资源自行结束时可提前摘掉）。
  // 作用域已释放时：立即执行一次 fn（避免注册即泄漏），并返回空操作。
  function onDispose(fn) {
    if (typeof fn !== 'function') return () => {};
    if (disposed) {
      try { fn(); } catch (e) { console.warn('[scope:' + name + '] late dispose failed:', e); }
      return () => {};
    }
    disposalQueue.push(fn);
    return () => {
      const i = disposalQueue.indexOf(fn);
      if (i >= 0) disposalQueue.splice(i, 1);
    };
  }

  const resources = {
    onDispose,

    setInterval(fn, ms) {
      if (disposed) return null;
      const id = setInterval(fn, ms);
      onDispose(() => clearInterval(id));
      return id;
    },

    setTimeout(fn, ms) {
      if (disposed) return null;   // 已释放：不再调度新工作（递归轮询自然停止）
      let id = null;
      const off = onDispose(() => { if (id !== null) clearTimeout(id); });
      id = setTimeout(() => { off(); fn(); }, ms);
      return id;
    },

    on(target, type, handler, options) {
      if (disposed || !target) return handler;
      target.addEventListener(type, handler, options);
      onDispose(() => target.removeEventListener(type, handler, options));
      return handler;
    },

    // WebSocket 句柄：形如 api.connectWebSocket() 返回值（{ close() }）
    ws(handle) {
      if (handle && typeof handle.close === 'function') onDispose(() => handle.close());
      return handle;
    },

    // body 级浮层：挂到 body 并登记，切页即移除（避免残留在别的页面上）。
    // 一步到位是有意的——直接写 document.body.appendChild 容易只记得挂、
    // 忘记在切页时摘掉；漏掉挂载则浮层永远不出现。
    overlay(el, parent) {
      if (!el) return el;
      const host = parent || document.body;
      if (host && !el.isConnected) host.appendChild(el);
      onDispose(() => { try { el.remove(); } catch {} });
      return el;
    },
  };

  return {
    name,
    tabId,          // 供页面拼「按标签命名」的存储键（两个标签不能共用一个键）
    resources,

    // ---- 标签页状态快照（M1：重建式标签切回时恢复页面上下文）----
    // 页面挂载时用 defaults 声明状态形状并取回上次的快照：
    //   let state = scope.restoreState({ page: 1, query: '', ... });
    // 只合并 defaults 里出现过的键，避免旧快照注入未知字段。
    restoreState(defaults) {
      const out = { ...(defaults || {}) };
      if (!snapshot) return out;
      for (const k of Object.keys(out)) {
        if (Object.prototype.hasOwnProperty.call(snapshot, k)) out[k] = snapshot[k];
      }
      return out;
    },

    // 注册状态提取器：app.js 在切走/关闭标签前取一次快照（惰性，不做周期性开销）。
    // 传投影而不是整个状态对象，可避免把列表缓存也存进快照。
    persistState(getter) {
      stateGetter = typeof getter === 'function' ? getter : null;
    },

    // 供 app.js 调用：取当前状态的 JSON 快照（取不到就返回 null，此时保留标签上的旧快照）
    takeSnapshot() {
      if (!stateGetter) return null;
      try {
        return JSON.parse(JSON.stringify(stateGetter()));
      } catch (e) {
        console.warn('[scope:' + name + '] snapshot failed:', e);
        return null;
      }
    },

    get disposed() { return disposed; },
    dispose() {
      if (disposed) return;
      disposed = true;
      stateGetter = null;
      // 逆序释放：后注册的多依赖先生成的资源（如先建 WS 再注册其重连定时器）
      const queue = disposalQueue;
      disposalQueue = [];
      for (let i = queue.length - 1; i >= 0; i--) {
        try { queue[i](); } catch (e) { console.warn('[scope:' + name + '] cleanup failed:', e); }
      }
    },
  };
}
