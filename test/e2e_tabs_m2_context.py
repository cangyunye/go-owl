"""E2E: 标签页 M2——面板随标签、嵌套上下文入路由、详情页可开新标签

覆盖：
A. 节点页点分组 → 筛进 URL（?group=）且标签标题带层级后缀
B. 刷新页面 → 分组上下文从 URL 恢复（不再只靠内存快照）
C. 两个节点标签各自分组：URL/标题互不相同，切换互不干扰
D. 进节点详情（/nodes/:id）→ 面板被清空（不再残留上一页的「节点分组」）
E. 中键点列表行 → 新标签打开节点详情，原标签 URL 与上下文不变
F. 面板标题随页面切换（节点分组 → 剧本分类）
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
GROUP = 'e2e-big'


def launch(p):
    for kwargs in ({}, {'channel': 'msedge'}, {'channel': 'chrome'}):
        try:
            return p.chromium.launch(headless=True, **kwargs)
        except Exception:
            continue
    raise RuntimeError('no chromium/msedge/chrome available')


def active_title(page):
    return page.eval_on_selector('#tabbar .tab.active .tab-title', 'e => e.textContent.trim()')


def titles(page):
    return page.eval_on_selector_all('#tabbar .tab .tab-title', 'els => els.map(e => e.textContent.trim())')


def url(page):
    return page.evaluate('location.pathname + location.search')


def click_view(page, view):
    page.eval_on_selector('.nav-item[data-view="%s"]' % view, 'e => e.click()')


def panel_group(page, group):
    page.eval_on_selector('#panelList .group-chip[data-group="%s"]' % group, 'e => e.click()')
    page.wait_for_timeout(1500)


def main():
    with sync_playwright() as p:
        browser = launch(p)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
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
        page.wait_for_timeout(2200)
        page.evaluate("localStorage.removeItem('owl-tabs')")   # 从干净的单标签开始
        page.goto(BASE + '/nodes')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2500)

        # A. 分组进 URL + 标题层级
        panel_group(page, GROUP)
        assert ('group=' + GROUP) in url(page), 'A: 分组应写进 URL，实际 %r' % url(page)
        assert GROUP in active_title(page), 'A: 标签标题应带分组层级，实际 %r' % active_title(page)
        print('A PASS URL=%s 标题=%r' % (url(page), active_title(page)))

        # B. 刷新恢复上下文（来自 URL，不是内存）
        page.reload()
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2500)
        assert ('group=' + GROUP) in url(page), 'B: 刷新后 URL 应保留分组'
        assert GROUP in active_title(page), 'B: 刷新后标签标题应保留分组，实际 %r' % active_title(page)
        selected = page.eval_on_selector_all(
            '#panelList .group-chip.selected[data-group]', 'els => els.map(e => e.dataset.group)')
        assert GROUP in selected, 'B: 刷新后分组筛选应仍然生效，实际选中 %r' % selected
        print('B PASS 刷新后分组仍在: %r' % selected)

        # C. 第二个节点标签（Ctrl 点击）用另一个上下文
        page.eval_on_selector('.nav-item[data-view="nodes"]',
                              'e => e.dispatchEvent(new MouseEvent("click", {ctrlKey: true, bubbles: true}))')
        page.wait_for_timeout(2000)
        assert len(titles(page)) == 2, 'C: 应有两个标签，实际 %r' % titles(page)
        assert url(page) == '/nodes', 'C: 新标签应是干净上下文，实际 %r' % url(page)
        t2 = active_title(page)
        page.eval_on_selector('#tabbar .tab:nth-child(1)', 'e => e.click()')
        page.wait_for_timeout(2000)
        t1 = active_title(page)
        assert t1 != t2, 'C: 两个标签标题应不同（各自上下文），实际 %r / %r' % (t1, t2)
        assert GROUP in t1 and GROUP not in t2, 'C: 标题层级串了: %r / %r' % (t1, t2)
        print('C PASS 两标签标题: %r / %r' % (t1, t2))

        # D. 详情页清空面板
        row_id = page.eval_on_selector('#node-list tr[data-toggle]', 'e => e.dataset.toggle')
        page.evaluate("history.pushState(null,'','/nodes/%s'); window.dispatchEvent(new PopStateEvent('popstate'))" % row_id)
        page.wait_for_timeout(2000)
        panel_html = page.eval_on_selector('#panelList', 'e => e.innerHTML')
        assert 'group-chip' not in panel_html, 'D: 详情页面板应被清空，实际仍有分组面板'
        assert active_title(page) == '节点详情', 'D: 标签标题应为节点详情，实际 %r' % active_title(page)
        print('D PASS 详情页面板已清空，标题=%r' % active_title(page))

        # E. 中键点行 → 新标签打开详情
        page.evaluate("history.pushState(null,'','/nodes'); window.dispatchEvent(new PopStateEvent('popstate'))")
        page.wait_for_timeout(2200)
        before = len(titles(page))
        page.eval_on_selector('#node-list tr[data-toggle]',
                              'e => e.dispatchEvent(new MouseEvent("auxclick", {button: 1, bubbles: true}))')
        page.wait_for_timeout(2000)
        assert len(titles(page)) == before + 1, 'E: 中键应新开一个标签: %r -> %r' % (before, titles(page))
        assert page.evaluate('location.pathname').startswith('/nodes/'), \
            'E: 新标签应停在节点详情，实际 %r' % url(page)
        print('E PASS 中键新标签: %r' % titles(page))

        # F. 面板标题随页面
        click_view(page, 'playbooks')
        page.wait_for_timeout(2200)
        ptitle = page.eval_on_selector('#panelTitle', 'e => e.textContent.trim()')
        assert '剧本分类' in ptitle, 'F: 剧本页面板标题应为剧本分类，实际 %r' % ptitle
        print('F PASS 面板标题=%r' % ptitle)

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
