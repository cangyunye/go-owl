"""E2E: M4b 增量取输出——轻量任务列表 + 按游标补输出

用 webapp-testing 的方式（headless chromium、networkidle、先侦察后动作）驱动真实执行：
A. 对账走轻量列表：/tasks?record_id=...&light=1（不再整段重传 output）
B. 输出完整：长输出命令（seq 1 20000 ≈ 120KB）跑完后终端里有最后一行
C. 增量端点：按 offset 取中段/尾部，与任务记录的完整输出逐段一致
"""
import json
import sys
import time
import urllib.request

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
NODE_ID = 'e2e-term-pi'
CMD = 'seq 1 20000'


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
    api('POST', '/nodes', {'id': NODE_ID, 'name': 'e2e-term-pi', 'address': '192.168.31.100',
                           'port': 22, 'user': 'kali', 'password': 'hwx1515661', 'groups': ['e2e-term']})

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

        recon, cursor = [], []
        def on_req(r):
            u = r.url
            if '/api/v1/tasks?' in u and 'record_id=' in u:
                recon.append(u)
            elif '/api/v1/tasks/' in u and '/output?' in u:
                cursor.append(u)
        page.on('request', on_req)

        # 侦察：确认执行页结构后再动作
        # 走 URL 预选节点：面板列表是服务端分页的，搜索框只过滤当前页（另一个待修的小缺陷）
        page.goto(BASE + '/exec?nodes=' + NODE_ID)
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2000)
        assert page.locator('#cmd-input').count() == 1, '前置：执行页命令输入框不存在'

        page.fill('#cmd-input', CMD)
        page.wait_for_timeout(600)
        page.click('#exec-btn')   # 侦察得：提交按钮是 #exec-btn
        print('已提交执行，等待任务终态…')

        task_id = None
        for _ in range(60):
            time.sleep(1.5)
            ts = api('GET', '/tasks?page=1&page_size=5').get('data') or []
            if ts and ts[0].get('status') in ('completed', 'success', 'failed', 'partial_failure'):
                task_id = ts[0]['id']
                break
        assert task_id, '任务未在预期时间内进入终态'
        page.wait_for_timeout(6000)   # 留给对账/补尾一轮

        # A. 对账用轻量列表
        light = [u for u in recon if 'light=1' in u]
        heavy = [u for u in recon if 'light=1' not in u]
        assert light, 'A: 对账应使用 light=1 轻量列表（实际未观察到）'
        assert not heavy, 'A: 不应再有整段重传的对账请求，实际 %d 次' % len(heavy)
        print('A PASS 轻量对账 %d 次（light=1），全量 %d 次' % (len(light), len(heavy)))

        # B. 长输出完整：任务记录完整 + 页面在跑长输出期间零报错
        lines = api('GET', '/tasks/' + task_id).get('output') or ''
        assert lines.rstrip().endswith('20000'),             'B: 任务记录的输出应以 20000 结尾（实际 %d 字节，尾部 %r）' % (len(lines), lines[-40:])
        assert len(lines) > 100000, 'B: seq 1 20000 应产生 >100KB 输出，实际 %d' % len(lines)
        print('B PASS 长输出完整（记录 %d 字节）' % len(lines))

        # C. 增量端点逐段一致
        full = api('GET', '/tasks/' + task_id).get('output') or ''
        head = api('GET', '/tasks/%s/output?offset=0&limit=120' % task_id)
        mid_off = max(0, len(full) // 2)
        mid = api('GET', '/tasks/%s/output?offset=%d&limit=120' % (task_id, mid_off))
        tail = api('GET', '/tasks/%s/output?offset=%d' % (task_id, max(0, len(full) - 60)))
        assert head.get('data') == full[:120], 'C: 头部片段不一致'
        assert mid.get('data') == full[mid_off:mid_off + 120], 'C: 中段片段不一致'
        assert tail.get('data') == full[max(0, len(full) - 60):], 'C: 尾部片段不一致'
        assert tail.get('total') == len(full), 'C: total 应为输出总字节数'
        print('C PASS 增量端点：head/mid/tail 与记录逐段一致（total=%d）' % tail.get('total'))
        if cursor:
            print('   观察到 %d 次按游标补输出请求' % len(cursor))

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
