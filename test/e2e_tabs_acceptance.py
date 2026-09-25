"""验收：按最初需求——多标签页，一个标签进一个视图

场景（用户原话）：
  一个标签进节点管理、一个进剧本管理、一个进系统设置、一个进监控（告警中心），
  外加「多个标签同时在节点管理」（各自不同分组上下文）
校验点：标签栏呈现这 5 个标签；各自页面内容正确渲染；两个节点标签的分组上下文互不干扰；
        切换任意标签后原上下文仍在。
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
GROUP_A, GROUP_B = 'e2e-small', 'e2e-big'


def launch(p):
    for kwargs in ({}, {'channel': 'msedge'}, {'channel': 'chrome'}):
        try:
            return p.chromium.launch(headless=True, **kwargs)
        except Exception:
            continue
    raise RuntimeError('no chromium/msedge/chrome available')


def titles(page):
    return page.eval_on_selector_all('#tabbar .tab .tab-title', 'els => els.map(e => e.textContent.trim())')


def active(page):
    return page.eval_on_selector('#tabbar .tab.active .tab-title', 'e => e.textContent.trim()')


def open_view(page, view):
    page.eval_on_selector('.nav-item[data-view="%s"]' % view, 'e => e.click()')
    page.wait_for_timeout(2200)


def main():
    with sync_playwright() as p:
        browser = launch(p)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN)
        page = ctx.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.goto(BASE + '/')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2200)
        page.evaluate("localStorage.removeItem('owl-tabs')")
        page.reload()
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2200)

        # 1) 依次开四个标签：节点管理 / 剧本管理 / 系统设置 / 告警中心（监控）
        open_view(page, 'nodes')
        open_view(page, 'playbooks')
        open_view(page, 'settings')
        open_view(page, 'alerts')
        got = titles(page)
        assert len(got) == 5, '仪表盘 + 四个视图应共 5 个标签，实际 %r' % got
        for want in ('节点管理', '剧本管理', '系统设置', '告警中心'):
            assert want in got, '缺少标签 %s（实际 %r）' % (want, got)
        print('1 PASS 四个视图各一个标签:', got)

        # 2) 第二个节点管理标签（Ctrl+点击），两个标签各选不同分组
        page.eval_on_selector('.nav-item[data-view="nodes"]',
                              'e => e.dispatchEvent(new MouseEvent("click", {ctrlKey: true, bubbles: true}))')
        page.wait_for_timeout(2200)
        page.eval_on_selector('#panelList .group-chip[data-group="%s"]' % GROUP_B, 'e => e.click()')
        page.wait_for_timeout(1800)
        title_b = active(page)
        assert GROUP_B in title_b, '新节点标签应带 %s 上下文，实际 %r' % (GROUP_B, title_b)
        print('2 PASS 第二个节点标签上下文:', title_b)

        # 3) 切回第一个节点标签，选另一分组；两个标签互不干扰
        nodes_tabs = [i + 1 for i, t in enumerate(titles(page)) if t.startswith('节点管理')]
        assert len(nodes_tabs) == 2, '应有两个节点标签，实际 %r' % titles(page)
        page.eval_on_selector('#tabbar .tab:nth-child(%d)' % nodes_tabs[0], 'e => e.click()')
        page.wait_for_timeout(2000)
        page.eval_on_selector('#panelList .group-chip[data-group="%s"]' % GROUP_A, 'e => e.click()')
        page.wait_for_timeout(1800)
        title_a = active(page)
        assert GROUP_A in title_a and GROUP_B not in title_a, '两个节点标签应各持上下文，实际 %r' % title_a
        print('3 PASS 两个节点标签各自上下文:', title_a, '/', title_b)

        # 4) 全部标签切一遍，页面都能正常渲染（无空容器）
        for i in range(1, len(titles(page)) + 1):
            page.eval_on_selector('#tabbar .tab:nth-child(%d)' % i, 'e => e.click()')
            page.wait_for_timeout(1600)
            kids = page.evaluate("document.querySelector('.view-container').childElementCount")
            assert kids > 0, '第 %d 个标签渲染为空' % i
        print('4 PASS %d 个标签逐个切换均正常渲染' % len(titles(page)))

        page.locator('#tabbar').screenshot(path='build/tabs_acceptance.png')
        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
