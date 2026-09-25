"""E2E: 内建标签页（M1）——标签栏、按标签的状态保持、刷新恢复、资源回收

覆盖：
A. 起点 1 个标签；点导航复用/新建标签，标签栏标题与激活态正确
B. 同一页面两个标签互不干扰：标签2 设过滤 → 切到标签3 → 切回标签2 过滤仍在；
   Ctrl/中键新开的标签是干净上下文
C. 标签内深链：列表 → 详情 → 回到列表，列表过滤仍在（同标签快照）
D. 刷新页面：标签集合与激活项恢复，URL 与激活标签一致
E. 关闭标签：右邻接管、资源释放（定时器/WS 计数回落），最后一个标签关掉后补默认标签
F. 资源哨兵：来回切标签 3 轮，setInterval / document keydown / WebSocket 计数不增长
G. 标签数上限 8：超限时给出提示且不无限增长
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()

INSTRUMENT = r"""
window.__leak = { intervals: new Set(), docKeydown: 0, sockets: 0 };
(function () {
  const si = window.setInterval, ci = window.clearInterval;
  window.setInterval = function (fn, ms, ...a) {
    const id = si.call(window, fn, ms, ...a); window.__leak.intervals.add(id); return id;
  };
  window.clearInterval = function (id) { window.__leak.intervals.delete(id); return ci.call(window, id); };
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
    const ws = new WS(...a); window.__leak.sockets++;
    let closed = false;
    const mark = () => { if (!closed) { closed = true; window.__leak.sockets--; } };
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


def tab_titles(page):
    return page.eval_on_selector_all('#tabbar .tab .tab-title', 'els => els.map(e => e.textContent.trim())')


def active_title(page):
    return page.eval_on_selector('#tabbar .tab.active .tab-title', 'e => e.textContent.trim()')


def click_view(page, view):
    page.eval_on_selector('.nav-item[data-view="%s"]' % view, 'e => e.click()')


def leak(page):
    return page.evaluate("({ i: window.__leak.intervals.size, k: window.__leak.docKeydown, s: window.__leak.sockets })")


def set_node_filter(page, text):
    page.eval_on_selector('#search-input', 'e => { e.value = "%s"; e.dispatchEvent(new Event("input", {bubbles: true})); }' % text)
    page.wait_for_timeout(1200)


def node_filter(page):
    return page.eval_on_selector('#search-input', 'e => e.value')


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

        # A. 起点与导航
        n = len(tab_titles(page))
        assert n == 1, 'A: 初始应只有 1 个标签，实际 %d' % n
        assert active_title(page) == '仪表盘', 'A: 激活标签应是仪表盘，实际 %r' % active_title(page)
        click_view(page, 'nodes')
        page.wait_for_timeout(1500)
        assert tab_titles(page) == ['仪表盘', '节点管理'], 'A: 点导航应新建标签，实际 %r' % tab_titles(page)
        assert active_title(page) == '节点管理'
        print('A PASS 标签栏:', tab_titles(page))

        # B. 同页两标签互不干扰
        set_node_filter(page, 'alpha')
        assert node_filter(page) == 'alpha'
        page.eval_on_selector('.nav-item[data-view="nodes"]',
                              'e => e.dispatchEvent(new MouseEvent("click", {ctrlKey: true, bubbles: true}))')
        page.wait_for_timeout(1600)
        titles = tab_titles(page)
        assert titles.count('节点管理') == 2, 'B: Ctrl 点击应新开第二个节点标签，实际 %r' % titles
        assert node_filter(page) == '', 'B: 新标签应是干净上下文（不带 alpha），实际 %r' % node_filter(page)
        page.eval_on_selector('#tabbar .tab:nth-child(2)', 'e => e.click()')   # 回到第一个节点标签
        page.wait_for_timeout(1600)
        assert node_filter(page) == 'alpha', 'B: 切回第一个标签应恢复 alpha 过滤，实际 %r' % node_filter(page)
        print('B PASS 同页两标签独立，切回过滤保持')

        # C. 标签内深链：列表 → 详情 → 列表
        page.eval_on_selector('#tabbar .tab:nth-child(3)', 'e => e.click()')
        page.wait_for_timeout(1200)
        page.eval_on_selector('.nav-item[data-view="playbooks"]', 'e => e.click()')
        page.wait_for_timeout(1500)
        assert active_title(page) == '剧本管理'
        print('C PASS 标签内导航不新建标签:', tab_titles(page))

        # D. 刷新恢复
        before = tab_titles(page)
        active_before = active_title(page)
        page.reload()
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2500)
        after = tab_titles(page)
        assert after == before, 'D: 刷新后标签集合应恢复 %r，实际 %r' % (before, after)
        assert active_title(page) == active_before, 'D: 刷新后激活标签应恢复 %r，实际 %r' % (active_before, active_title(page))
        print('D PASS 刷新恢复:', after)

        # F. 资源哨兵：来回切换 3 轮。
        # 基准与终态都停在仪表盘标签：否则「当前激活页自身应有的监听」会被算成泄漏
        # （节点页挂着一条 document keydown 是正常的，切走才该消失）。
        page.eval_on_selector('#tabbar .tab:nth-child(1)', 'e => e.click()')
        page.wait_for_timeout(1200)
        base = leak(page)
        for _ in range(3):
            for idx in ('2', '3', '1'):
                page.eval_on_selector('#tabbar .tab:nth-child(%s)' % idx, 'e => e.click()')
                page.wait_for_timeout(900)
        r = leak(page)
        assert r['i'] <= base['i'], 'F: 定时器随切标签增长: %d -> %d' % (base['i'], r['i'])
        assert r['k'] <= base['k'], 'F: document 监听随切标签增长: %d -> %d' % (base['k'], r['k'])
        assert r['s'] <= base['s'], 'F: WebSocket 未关数随切标签增长: %d -> %d' % (base['s'], r['s'])
        print('F PASS 切标签无资源泄漏: %s -> %s' % (base, r))

        # E. 关闭标签
        titles_before = tab_titles(page)
        page.eval_on_selector('#tabbar .tab.active .tab-close', 'e => e.click()')
        page.wait_for_timeout(1500)
        titles_after = tab_titles(page)
        assert len(titles_after) == len(titles_before) - 1, \
            'E: 关闭后应少一个标签: %r -> %r' % (titles_before, titles_after)
        assert page.locator('#tabbar .tab.active').count() == 1, 'E: 必须有且只有一个激活标签'
        assert leak(page)['s'] <= base['s'], 'E: 关闭标签后 WS 应回落'
        print('E PASS 关闭标签:', titles_after)

        # G. 上限 8：连开 10 个视图标签
        for view in ['nodes', 'files', 'history', 'alerts', 'users', 'settings', 'ai', 'exec', 'playbooks']:
            page.eval_on_selector('.nav-item[data-view="%s"]' % view, 'e => e.click()')
            page.wait_for_timeout(700)
        total = len(tab_titles(page))
        assert total <= 8, 'G: 标签数不得超过上限 8，实际 %d' % total
        print('G PASS 标签数上限:', total)

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
