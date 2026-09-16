"""E2E: 节点分组统计正确性

数据：140 个节点，其中 big-group 90 个成员（按名称排序只有 10 个落在前 20）。
修复前：分组徽标只统计服务端回落后的前 20 个节点 → big-group 显示 10。
修复后：按 meta.total 翻页拉全量 → big-group 显示 90。
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
TRUTH = 90


def main():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context(viewport={'width': 1280, 'height': 900})
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN
        )
        page = ctx.new_page()
        page.goto(BASE + '/nodes')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2500)

        chip = page.locator('#panelList .group-chip[data-group="big-group"]')
        chip.wait_for(state='visible', timeout=8000)
        count = page.locator('#panelList .group-chip[data-group="big-group"] .count').inner_text()
        assert count == str(TRUTH), 'big-group 徽标应显示 %d，实际 %r' % (TRUTH, count)
        print('PASS big-group 徽标计数 =', count)

        total_text = page.inner_text('#panelList .group-chip[data-group="web"] .count')
        print('PASS web 分组徽标计数 =', total_text)
        assert total_text.isdigit() and int(total_text) > 0

        page.screenshot(path='test/e2e_issue5_final.png')
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
