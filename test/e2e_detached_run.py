"""E2E: 分离方式运行（UI 开关 + 查看分离输出）

A. 执行页勾选「分离方式运行」后提交，请求体带 detached
B. 提交后出现「查看分离输出」按钮
C. 点击按钮：读到进程状态与日志（含脚本打印的标记行）
D. 再等一会儿点击：日志继续增长（说明进程在页面之外继续跑）
"""
import json
import sys
import urllib.request

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
NODE_ID = 'e2e-term-pi'
CMD = 'for i in 1 2 3 4 5 6; do echo det-tick $i; sleep 2; done'


def api(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + '/api/v1' + path, data=data, method=method, headers={
        'Authorization': 'Bearer ' + TOKEN, 'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req) as r:
            return json.loads(r.read().decode() or '{}')
    except urllib.error.HTTPError as e:
        if e.code in (400, 404, 409):
            return {}
        raise


def launch(p):
    for kwargs in ({}, {'channel': 'msedge'}, {'channel': 'chrome'}):
        try:
            return p.chromium.launch(headless=True, **kwargs)
        except Exception:
            continue
    raise RuntimeError('no chromium/msedge/chrome available')


def main():
    api('POST', '/nodes', {'id': NODE_ID, 'name': 'pi', 'address': '192.168.31.100',
                           'port': 22, 'user': 'kali', 'password': 'hwx1515661'})
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
        bodies = []
        page.on('request', lambda r: bodies.append(r.post_data) if '/api/v1/exec' in r.url and r.method == 'POST' else None)

        page.goto(BASE + '/exec?nodes=' + NODE_ID)
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2200)
        assert page.locator('#detached-run').count() == 1, 'A: 应存在「分离方式运行」开关'
        assert not page.locator('#detached-log-btn').is_visible(), 'A: 未运行时按钮应隐藏'

        page.fill('#cmd-input', CMD)
        page.evaluate("document.getElementById('detached-run').click()")   # 样式化开关：原生 checkbox 不可见，点它等价于点轨道
        assert page.evaluate("document.getElementById('detached-run').checked") is True, 'A: 开关应已勾选'
        print('A0 PASS 开关已勾选')
        page.click('#exec-btn')
        page.wait_for_timeout(4000)
        body = bodies[-1] or ''
        assert '"detached":true' in body.replace(' ', ''), 'A: 请求体应带 detached，实际 %r' % body[:160]
        print('A PASS 请求体带 detached')

        page.wait_for_selector('#detached-log-btn', state='visible', timeout=8000)
        print('B PASS 「查看分离输出」按钮已出现')

        page.click('#detached-log-btn')
        page.wait_for_timeout(2500)
        t1 = page.inner_text('#term-body')
        assert '分离进程' in t1 and 'det-tick' in t1, 'C: 应看到进程状态与日志，实际末段 %r' % t1[-120:]
        print('C PASS 读到分离进程状态与日志')

        page.wait_for_timeout(6000)
        page.click('#detached-log-btn')
        page.wait_for_timeout(2500)
        t2 = page.inner_text('#term-body')
        assert 'det-tick 5' in t2 or 'det-tick 6' in t2, 'D: 日志应继续增长（说明进程在页面之外继续跑）'
        print('D PASS 日志继续增长（进程在页面之外运行）')

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()
    api('DELETE', '/nodes/' + NODE_ID)
    print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
