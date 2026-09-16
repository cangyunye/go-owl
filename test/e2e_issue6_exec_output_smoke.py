"""E2E 冒烟：命令执行页输出链路改动后页面正常（无运行时报错，终端可用）

输出流本身依赖真实 SSH 节点，此脚本覆盖前端改动不破坏页面的部分：
A. /exec 加载无 console 错误，终端区与实时输出等待提示正常
B. 执行一个针对不可达节点的命令：前端能完成"提交 → 终态 → 日志区"全流程，
   终态对账逻辑（receivedLines/taskUpdates）不抛异常
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

        page.goto(BASE + '/exec')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(1500)

        assert page.locator('#term-body').count() == 1, 'A: 终端区应渲染'
        assert page.locator('#command').count() == 1 or page.locator('#cmd-input').count() == 1, 'A: 命令输入框应存在'
        assert page.locator('#select-all-btn').count() == 1, 'A: 节点选择面板应渲染'
        print('A PASS 页面渲染正常')

        # 选一个节点执行（节点不可达：SSH 失败路径也要走完终态流程）
        page.locator('#panel-node-list .node-chip').first.click()
        page.wait_for_timeout(300)
        page.fill('#cmd-input', 'echo e2e-output-smoke')
        page.click('#exec-btn')
        page.wait_for_timeout(4000)

        term = page.inner_text('#term-body')
        assert '任务' in term or '执行' in term or '失败' in term, 'B: 终端应有执行反馈: %r' % term[:200]
        js_errors = [e for e in errors if 'favicon' not in e]
        assert not js_errors, 'B: 不应有 console/运行时报错: %r' % js_errors[:3]
        print('B PASS 提交流程无异常，终端输出:', ' | '.join(term.split('\n')[:3]))

        page.screenshot(path='test/e2e_issue6_final.png')
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
