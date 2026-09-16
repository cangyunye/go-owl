"""E2E: 传输记录区分度与提交措辞（问题2）

场景：
A. 记录行显示方向徽标（上传）、绝对时间、状态，可与旧同名记录区分
B. 中转站批量传输：先全量确认（接受），提交后提示"已提交…后台进行"，不得声称"完成"
"""
import re
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()

dialogs = []
accept_next = []


def on_dialog(d):
    dialogs.append({'type': d.type, 'message': d.message})
    do_accept = accept_next.pop(0) if accept_next else False
    if do_accept:
        d.accept()
    else:
        d.dismiss()


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
        page.on('dialog', on_dialog)

        page.goto(BASE + '/files')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(1500)

        # ---------- Case A: 记录行方向徽标 + 绝对时间 ----------
        row = page.locator('#transfer-list .task-item').first
        text = row.inner_text()
        assert '上传' in text, 'A: 记录行缺少方向徽标: %r' % text
        assert 'debug.log' in text, 'A: 记录行缺少文件名: %r' % text
        assert re.search(r'\d{1,4}/\d{1,2}/\d{1,4}\s+\d{1,2}:\d{2}', text), 'A: 记录行缺少绝对时间: %r' % text
        assert re.search(r'失败|传输中|已完成|部分成功', text), 'A: 记录行缺少状态: %r' % text
        print('A PASS 记录行:', ' | '.join(text.split('\n')))

        # ---------- Case B: 批量传输提交措辞 ----------
        page.click('#staging-multi-btn')
        page.wait_for_timeout(500)
        page.locator('.staging-checkbox').first.check()
        page.wait_for_timeout(300)
        dialogs.clear()
        accept_next.append(True)  # 接受全量传输 confirm
        page.click('#staging-batch-btn')
        page.wait_for_timeout(2500)
        assert len(dialogs) >= 2, 'B: 应先出 confirm 再出提交提示，实际 %d 个: %r' % (len(dialogs), dialogs)
        confirm, alert = dialogs[0], dialogs[1]
        assert confirm['type'] == 'confirm' and '未选择任何分组/标签' in confirm['message'], 'B: 缺少全量确认: %r' % confirm
        assert '51 个节点' in confirm['message'], 'B: 确认框未统计到 51 个节点: %r' % confirm['message']
        assert alert['type'] == 'alert', 'B: 第二个应为 alert: %r' % alert
        assert alert['message'].startswith('已提交'), 'B: 措辞应为"已提交": %r' % alert['message']
        assert '后台进行' in alert['message'], 'B: 应提示后台进行: %r' % alert['message']
        assert '完成' not in alert['message'], 'B: 不得声称完成: %r' % alert['message']
        print('B PASS confirm:', confirm['message'].replace('\n', ' | '))
        print('B PASS alert:', alert['message'])
        page.wait_for_timeout(1200)

        page.screenshot(path='test/e2e_issue2_final.png', full_page=True)
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
