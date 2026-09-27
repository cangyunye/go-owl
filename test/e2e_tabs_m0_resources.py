"""E2E: 页面资源泄漏哨兵（M0）

在多页 SPA 里反复切页（不刷新），用注入的计数器统计：
  intervals —— 活动 setInterval 数
  docKeydown —— document 上 keydown 监听净增数
  sockets   —— 未关闭的 WebSocket 数
断言它们不随「访问次数」增长，即切走的页面必须把资源全部释放。

修复前（M0 之前）预期失败：
  files 页每进一次多一个 5s 轮询（stopAutoRefresh 全仓无调用点）；
  nodes / settings 页每进一次多一条 document keydown（只加不移）。
修复后：两轮遍历后计数回到基线。
"""
import json
import sys
import urllib.request

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
ALERT_TYPE = 'OWL-CUS-E2ENODE'


def api(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + '/api/v1' + path, data=data, method=method, headers={
        'Authorization': 'Bearer ' + TOKEN, 'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req) as r:
            return json.loads(r.read().decode() or '{}')
    except urllib.error.HTTPError as e:
        if e.code in (400, 409):  # 已存在等幂等冲突
            return {}
        raise

INSTRUMENT = r"""
window.__leak = { intervals: new Set(), docKeydown: 0, sockets: 0 };
(function () {
  const si = window.setInterval, ci = window.clearInterval;
  window.setInterval = function (fn, ms, ...a) {
    const id = si.call(window, fn, ms, ...a);
    window.__leak.intervals.add(id);
    return id;
  };
  window.clearInterval = function (id) {
    window.__leak.intervals.delete(id);
    return ci.call(window, id);
  };
  const ae = EventTarget.prototype.addEventListener, re = EventTarget.prototype.removeEventListener;
  EventTarget.prototype.addEventListener = function (t, fn, o) {
    if (this === document && t === 'keydown') window.__leak.docKeydown++;
    return ae.call(this, t, fn, o);
  };
  EventTarget.prototype.removeEventListener = function (t, fn, o) {
    if (this === document && t === 'keydown') window.__leak.docKeydown--;
    return re.call(this, t, fn, o);
  };
  const WS = window.WebSocket;
  function CountingWS(...a) {
    const ws = new WS(...a);
    window.__leak.sockets++;
    let closed = false;
    const mark = () => { if (!closed) { closed = true; window.__leak.sockets--; } };
    const close = ws.close.bind(ws);
    ws.close = function (...b) { mark(); return close(...b); };
    ws.addEventListener('close', mark);
    return ws;
  }
  CountingWS.CONNECTING = WS.CONNECTING;
  CountingWS.OPEN = WS.OPEN;
  CountingWS.CLOSING = WS.CLOSING;
  CountingWS.CLOSED = WS.CLOSED;
  CountingWS.prototype = WS.prototype;
  window.WebSocket = CountingWS;
})();
"""

# 走一遍会注册资源的页面（尽量覆盖全部一级页），最后一页用仪表盘（自身无定时器/WS）收尾
ROUND = ['files', 'nodes', 'settings', 'alerts', 'history', 'exec', 'playbooks', 'users', 'ai', 'dashboard']


def launch(p):
    for kwargs in ({}, {'channel': 'msedge'}, {'channel': 'chrome'}):
        try:
            return p.chromium.launch(headless=True, **kwargs)
        except Exception:
            continue
    raise RuntimeError('no chromium/msedge/chrome available')


def snapshot(page):
    return page.evaluate("({ i: window.__leak.intervals.size, k: window.__leak.docKeydown, s: window.__leak.sockets })")


def walk(page):
    for view in ROUND:
        page.eval_on_selector('.nav-item[data-view="%s"]' % view, 'e => e.click()')
        page.wait_for_timeout(1800)
        # 页面确实渲染了内容（渲染期抛错会留下空容器）
        kids = page.evaluate("document.querySelector('.view-container').childElementCount")
        assert kids > 0, '%s 页面渲染为空（可能有 JS 抛错）' % view


def main():
    with sync_playwright() as p:
        browser = launch(p)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
        ctx.add_init_script(INSTRUMENT)
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN
        )
        page = ctx.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.goto(BASE + '/')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2000)

        base = snapshot(page)
        print('基线        :', base)

        walk(page)
        r1 = snapshot(page)
        print('第一轮遍历后:', r1)

        walk(page)
        r2 = snapshot(page)
        print('第二轮遍历后:', r2)

        assert r2['i'] <= base['i'], \
            'setInterval 活动数随切页增长: 基线 %d -> %d（切走的页面没清定时器）' % (base['i'], r2['i'])
        print('PASS 定时器无泄漏: %d -> %d' % (base['i'], r2['i']))

        assert r2['k'] <= base['k'], \
            'document keydown 监听随切页增长: 基线 %d -> %d（只加不移）' % (base['k'], r2['k'])
        print('PASS document 监听无泄漏: %d -> %d' % (base['k'], r2['k']))

        assert r2['s'] <= base['s'], \
            'WebSocket 未关闭数随切页增长: 基线 %d -> %d' % (base['s'], r2['s'])
        print('PASS WebSocket 无泄漏: %d -> %d' % (base['s'], r2['s']))

        # ---- 浮层生命周期：该挂的挂得上，切页要摘掉 ----
        # 只登记不挂载 → 弹窗永不出现；只挂载不登记 → 残留到别的页面上。
        api('POST', '/alert-types', {
            'id': ALERT_TYPE, 'name': 'e2e-scope-overlay', 'category': 'custom',
            'default_severity': 'warn',
            'default_params': {'metric': 'custom_e2e_scope', 'op': '>', 'value': 1, 'duration': 1},
            'enabled': True, 'notifiable': False, 'builtin': False,
            'check_cmd': 'uptime', 'check_mode': 'exit_code', 'check_pattern': '',
            'scope_nodes': '', 'scope_groups': ''})
        page.eval_on_selector('.nav-item[data-view="alerts"]', 'e => e.click()')
        page.wait_for_timeout(1500)
        page.click('#toggle-config')
        page.wait_for_timeout(1200)
        page.locator('.at-debug[data-at-debug="%s"]' % ALERT_TYPE).first.click()
        page.wait_for_selector('#dbg-node', timeout=8000)
        print('PASS 浮层可正常打开（overlay 已挂到 body）')

        page.eval_on_selector('.nav-item[data-view="dashboard"]', 'e => e.click()')
        page.wait_for_timeout(1500)
        left = page.evaluate("document.body.querySelectorAll(':scope > .modal-overlay').length")
        assert left == 0, '切页后 body 上仍残留 %d 个浮层（overlay 未随作用域摘除）' % left
        print('PASS 浮层随切页摘除: 残留 %d' % left)

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()
        api('DELETE', '/alert-types/' + ALERT_TYPE)
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
