"""E2E: 传输记录列表稳定性（紧随其后的缺陷修复）

场景：
A. 60 条普通任务挤占 tasks 表最新 50 条时，任务详情 tab 仍显示传输任务（后端 SQL 过滤）
B. 拦断一次轮询请求（模拟瞬时错误）后，列表不得清空为"暂无传输"；恢复后正常刷新
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()


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
        page.goto(BASE + '/files')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(1500)

        # ---------- Case A: 任务详情不被普通任务挤占 ----------
        page.locator('#transfer-tabs .seg button[data-tab="tasks"]').click()
        page.wait_for_timeout(600)
        tasks_text = page.inner_text('#transfer-list')
        assert '暂无传输任务' not in tasks_text, 'A: 60 条普通任务不应挤走传输任务: %r' % tasks_text
        assert 'transfer:' in tasks_text, 'A: 任务详情应含传输任务: %r' % tasks_text
        print('A PASS 任务详情稳定')

        # ---------- Case B: 一次轮询失败不清空列表 ----------
        # 回到传输记录 tab，确认有记录
        page.locator('#transfer-tabs .seg button[data-tab="list"]').click()
        page.wait_for_timeout(600)
        assert 'a.tar' in page.inner_text('#transfer-list'), 'B: 前置条件失败，传输记录未显示'

        blocked = {'on': True}

        def maybe_abort(route):
            if blocked['on']:
                route.abort()
            else:
                route.continue_()

        page.route('**/api/v1/transfers*', maybe_abort)
        page.route('**/api/v1/transfer/records*', maybe_abort)

        # 等两个轮询周期（5s 一次），期间请求全部失败
        page.wait_for_timeout(11000)
        text_during_failure = page.inner_text('#transfer-list')
        assert '暂无传输记录' not in text_during_failure, 'B: 请求失败期间列表被清空: %r' % text_during_failure
        assert 'a.tar' in text_during_failure, 'B: 请求失败期间数据丢失: %r' % text_during_failure
        print('B PASS 失败期间列表保持:', ' | '.join(text_during_failure.split('\n')[:2]))

        # 恢复网络，等一个轮询周期，列表仍在且正常刷新
        blocked['on'] = False
        page.wait_for_timeout(6500)
        text_after = page.inner_text('#transfer-list')
        assert 'a.tar' in text_after, 'B: 恢复后列表应正常: %r' % text_after
        print('B PASS 恢复后列表正常')

        page.screenshot(path='test/e2e_issue4_final.png')
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
