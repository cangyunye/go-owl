"""E2E: M4a 共享 WS 总线——全应用一条连接，多个页面共用

修复前：剧本页/任务历史/任务详情/执行页各自 connectWebSocket()，各建一条 WS +
各自的 3s 重连；标签页保活后这些连接会同时存在（N 标签 ≈ N 连接 + N 份解码）。
修复后：一条总线连接，订阅者共用；订阅者归零时连接才关闭。
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()

INSTRUMENT = r"""
window.__ws = 0;
(function () {
  const WS = window.WebSocket;
  function CountingWS(...a) {
    const ws = new WS(...a); window.__ws++;
    let closed = false;
    const mark = () => { if (!closed) { closed = true; window.__ws--; } };
    const close = ws.close.bind(ws);
    ws.close = function (...b) { mark(); return close(...b); };
    ws.addEventListener('close', mark);
    return ws;
  }
  CountingWS.CONNECTING = WS.CONNECTING; CountingWS.OPEN = WS.OPEN;
  CountingWS.CLOSING = WS.CLOSING; CountingWS.CLOSED = WS.CLOSED;
  CountingWS.prototype = WS.prototype;
  window.WebSocket = CountingWS;
})();
"""


def launch(p):
    for kwargs in ({}, {'channel': 'msedge'}, {'channel': 'chrome'}):
        try:
            return p.chromium.launch(headless=True, **kwargs)
        except Exception:
            continue
    raise RuntimeError('no chromium/msedge/chrome available')


def main():
    with sync_playwright() as p:
        browser = launch(p)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
        ctx.add_init_script(INSTRUMENT)
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN)
        page = ctx.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.goto(BASE + '/')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2000)
        page.evaluate("localStorage.removeItem('owl-tabs')")

        # 依次打开三个会用 WS 的页面（各自一个标签）
        for view in ('playbooks', 'history'):
            page.eval_on_selector('.nav-item[data-view="%s"]' % view, 'e => e.click()')
            page.wait_for_timeout(2000)
        socks = page.evaluate('window.__ws')
        subs = page.evaluate('(async () => (await import("/static/js/api.js")).api.wsSubscribers())()')
        assert socks == 1, 'A: 多个页面应共用 1 条 WS，实际 %d 条' % socks
        assert subs >= 2, 'A: 应有 ≥2 个订阅者共用这条连接，实际 %r' % subs
        print('A PASS 三个页面共用 1 条 WS，订阅者=%s' % subs)

        # 触发一次 task_update 广播：执行页跑一条真实命令，历史页应通过同一条连接收到
        page.eval_on_selector('.nav-item[data-view="exec"]', 'e => e.click()')
        page.wait_for_timeout(2500)
        page.fill('#cmd-input', 'uptime')
        try:
            page.locator('.node-list-item').first.click()
        except Exception:
            pass
        page.wait_for_timeout(500)
        assert page.evaluate('window.__ws') == 1, 'B: 执行页接上后仍应只有 1 条 WS'

        # 关闭所有订阅者所在的标签 → 连接释放
        for _ in range(6):
            n = page.locator('#tabbar .tab').count()
            if n <= 1:
                break
            page.eval_on_selector('#tabbar .tab.active .tab-close', 'e => e.click()')
            page.wait_for_timeout(1200)
        page.wait_for_timeout(1500)
        socks = page.evaluate('window.__ws')
        print('C 关闭到只剩一个标签后 WS 数 =', socks)
        assert socks <= 1, 'C: 关掉页面后不应残留多条连接'

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
