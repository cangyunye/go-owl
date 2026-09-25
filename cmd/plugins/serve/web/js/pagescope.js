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

export function createPageScope(name = 'page') {
  let disposalQueue = [];
  let disposed = false;

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
    resources,
    get disposed() { return disposed; },
    dispose() {
      if (disposed) return;
      disposed = true;
      // 逆序释放：后注册的多依赖先生成的资源（如先建 WS 再注册其重连定时器）
      const queue = disposalQueue;
      disposalQueue = [];
      for (let i = queue.length - 1; i >= 0; i--) {
        try { queue[i](); } catch (e) { console.warn('[scope:' + name + '] cleanup failed:', e); }
      }
    },
  };
}
