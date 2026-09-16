"""E2E: 执行页对账/超时兜底/重试可见（A、C、D）

A. 批次执行期间页面按 record_id 轮询任务终态（对账兜底）
C. 清空 command_timeout 后提交：输入框回落默认 30，payload 仍带超时
D. 不可达节点的重试进度播报到实时终端（不再静默等待）
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()


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
        errors = []
        page.on('console', lambda m: errors.append(m.text) if m.type == 'error' else None)
        page.on('pageerror', lambda e: errors.append(str(e)))

        payloads = []
        page.on('request', lambda r: payloads.append(r.post_data or '')
                if '/api/v1/exec' in r.url and r.method == 'POST' else None)
        polled = []
        page.on('request', lambda r: polled.append(r.url) if 'record_id=' in r.url else None)

        page.goto(BASE + '/exec')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(1200)

        # ---------- C: 清空超时输入 ----------
        page.fill('#command-timeout', '')
        page.fill('#connect-timeout', '2')

        # ---------- 选目标节点并执行 ----------
        page.fill('#panel-node-search', 'e2e-refused')
        chip = page.locator('#panel-node-list .node-chip[data-id="e2e-refused"]')
        chip.wait_for(state='visible', timeout=8000)
        chip.click()
        page.fill('#cmd-input', 'echo retry-check')
        page.click('#exec-btn')
        page.wait_for_timeout(1000)

        restored = page.input_value('#command-timeout')
        assert restored == '30', 'C: 清空后应回落默认 30，实际 %r' % restored
        assert payloads, 'C: 未捕获到 exec 请求'
        assert 'command_timeout' in payloads[-1], 'C: payload 必须带 command_timeout: %r' % payloads[-1][:200]
        assert '30s' in payloads[-1], 'C: payload 应为默认 30s: %r' % payloads[-1][:200]
        print('C PASS 超时兜底生效，payload:', payloads[-1][:120])

        # ---------- A + D: 等待重试与终态对账 ----------
        term = ''
        for _ in range(30):  # 最多等 30s 让 4 次尝试跑完
            term = page.inner_text('#term-body')
            if '第 4/4 次尝试失败' in term or '执行失败' in term:
                break
            page.wait_for_timeout(1000)
        assert '次尝试失败' in term, 'D: 终端应显示重试进度: %r' % term[-400:]
        assert '后重试' in term, 'D: 应提示下次重试等待时长: %r' % term[-400:]
        assert polled, 'A: 应观察到按 record_id 的任务对账请求'
        assert '第 4/4 次尝试失败' in term or '执行失败' in term, \
            'A/D: 重试耗尽后应显示失败终态: %r' % term[-300:]
        js_errors = [e for e in errors if 'favicon' not in e]
        assert not js_errors, '不应有运行时报错: %r' % js_errors[:3]
        print('A PASS 对账轮询请求数:', len(polled))
        print('D PASS 终端输出尾部:', ' | '.join(term.split('\n')[-4:]))

        page.screenshot(path='test/e2e_issue7_final.png')
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
