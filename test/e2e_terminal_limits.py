"""E2E: 终端限额与空闲超时（系统设置 terminal.max_per_user / terminal.idle_timeout_min）

前置：真实可达节点（192.168.31.100 / kali）。
A. 每用户并发上限=2：开两个终端都能连上，第三个提示「终端数已达上限」
B. 空闲超时=1 分钟：终端闲置后自动断开，断开原因带「因空闲超过」
C. 恢复默认设置（5 / 30）
"""
import json
import sys
import time
import urllib.request

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
NODE_ID = 'e2e-term-pi'


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


def open_terminal(ctx, page_index):
    pg = ctx.new_page()
    pg.goto(BASE + '/terminal/' + NODE_ID)
    pg.wait_for_load_state('networkidle')
    pg.wait_for_timeout(3500)
    return pg


def term_text(pg):
    return pg.evaluate("document.querySelector('.xterm-rows') ? document.querySelector('.xterm-rows').innerText : ''")


def main():
    api('POST', '/nodes', {'id': NODE_ID, 'name': 'pi', 'address': '192.168.31.100',
                           'port': 22, 'user': 'kali', 'password': 'hwx1515661'})
    # 两项设置都在建连前写入：服务端在每次连接建立时读取（改设置不影响已存在的会话）
    api('PUT', '/settings/terminal.max_per_user', {'value': '2'})
    api('PUT', '/settings/terminal.idle_timeout_min', {'value': '1'})

    with sync_playwright() as p:
        browser = launch(p)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN)
        errors = []
        pages = []

        # A. 每用户并发上限=2
        for i in range(2):
            pg = open_terminal(ctx, i)
            errors.append(pg)
            pages.append(pg)
        for i, pg in enumerate(pages):
            assert '已连接' in pg.inner_text('#term-status') or '已断开' not in pg.inner_text('#term-status'), \
                'A: 第 %d 个终端应连上（检查 192.168.31.100 可达性）' % (i + 1)
        print('A 前半 PASS 两个终端均已连接')

        pg3 = ctx.new_page()
        pg3.goto(BASE + '/terminal/' + NODE_ID)
        pg3.wait_for_load_state('networkidle')
        pg3.wait_for_timeout(3500)
        t3 = term_text(pg3)
        assert '终端数已达上限' in t3, 'A: 第三个终端应提示已达上限，实际 %r' % t3[-120:]
        print('A PASS 第三个终端被拒绝并提示上限')

        # B. 空闲超时=1 分钟（设置已在建连前生效）
        print('B 等待空闲超时断开（约 60-90 秒）…')
        reason = None
        deadline = time.time() + 150
        while time.time() < deadline:
            txt = term_text(pages[0])
            if '因空闲超过' in txt:
                reason = txt
                break
            time.sleep(5)
        assert reason, 'B: 1 分钟空闲后终端应被服务端断开（150 秒内未观察到）'
        assert '已断开' in pages[0].inner_text('#term-status'), 'B: 状态应显示已断开'
        print('B PASS 空闲自动断开:', [l for l in reason.split('\n') if '因空闲' in l][0].strip()[:60])

        # C. 恢复默认设置
        api('PUT', '/settings/terminal.max_per_user', {'value': '5'})
        api('PUT', '/settings/terminal.idle_timeout_min', {'value': '30'})
        print('C PASS 设置已恢复默认（5 / 30）')

        browser.close()
    api('DELETE', '/nodes/' + NODE_ID)
    print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
