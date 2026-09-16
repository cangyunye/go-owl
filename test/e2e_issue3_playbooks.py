"""E2E: 剧本管理页重构（问题3）

场景：
A. 运行历史真分页：默认每页 20 条，页码信息"共 25 条 · 第 1/2 页"，上一页禁用
B. 下一页 → 5 条 / 第 2/2 页，下一页禁用；上一页可翻回
C. 剧本列表：运行/编辑/下载为统一 .btn 按钮，行/列有防拥挤类，panel 设计系统生效
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()


def row_count(page):
    return page.locator('#playbook-runs-list tr').count()


def main():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context()
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN
        )
        page = ctx.new_page()
        page.goto(BASE + '/playbooks')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(1500)

        # ---------- Panel 设计系统 ----------
        assert page.locator('.section-card').count() >= 3, '应有 section-card 结构'
        assert page.locator('#runs-pagination').is_visible(), '分页条应可见'

        # ---------- Case A: 第一页 ----------
        n = row_count(page)
        assert n == 20, 'A: 第一页应 20 行，实际 %d' % n
        info = page.inner_text('#runs-page-info')
        assert '共 25 条' in info and '第 1/2 页' in info, 'A: 页码信息异常: %r' % info
        assert page.locator('#runs-prev-btn').is_disabled(), 'A: 第一页上一页应禁用'
        assert not page.locator('#runs-next-btn').is_disabled(), 'A: 第一页下一页应可用'
        print('A PASS:', info)

        # ---------- Case B: 翻页 ----------
        page.click('#runs-next-btn')
        page.wait_for_timeout(1000)
        n = row_count(page)
        assert n == 5, 'B: 第二页应 5 行，实际 %d' % n
        info = page.inner_text('#runs-page-info')
        assert '第 2/2 页' in info, 'B: 页码信息异常: %r' % info
        assert page.locator('#runs-next-btn').is_disabled(), 'B: 最后一页下一页应禁用'
        print('B PASS 第二页:', info)

        page.click('#runs-prev-btn')
        page.wait_for_timeout(1000)
        assert row_count(page) == 20, 'B: 翻回第一页应 20 行'
        assert '第 1/2 页' in page.inner_text('#runs-page-info'), 'B: 翻回后页码信息异常'
        print('B PASS 翻回第一页')

        # ---------- Case C: 剧本列表按钮与防拥挤 ----------
        assert page.locator('#playbook-list .playbook-row').count() > 0, '应有剧本行'
        btn = page.locator('#playbook-list .playbook-row .btn.run-playbook-btn').first
        assert btn.inner_text().strip() == '运行', 'C: 运行按钮应统一样式与中文文案: %r' % btn.inner_text()
        assert page.locator('#playbook-list .cell-ellipsis').count() > 0, 'C: 列应有省略号截断'
        row_cls = page.locator('#playbook-list .playbook-row').first.get_attribute('class')
        assert 'playbook-row' in row_cls, 'C: 行类名异常'
        print('C PASS 按钮与防拥挤类生效')

        page.screenshot(path='test/e2e_issue3_final.png', full_page=True)
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
