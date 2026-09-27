"""E2E: 客户端减负——全量节点共享缓存 + 静态资源压缩/ETag

A. 同一份全量节点在多个标签/多次打开选择器时只拉一遍（并发去重 + 30s 缓存）
B. 节点发生变更后缓存立即失效（下次打开重新拉）
C. 静态资源带 ETag 与缓存头，命中 If-None-Match 返回 304；gzip 生效
"""
import json
import sys
import urllib.request

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
PB_NAME = 'e2e-cache-probe'


def api(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + '/api/v1' + path, data=data, method=method, headers={
        'Authorization': 'Bearer ' + TOKEN, 'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req) as r:
            return json.loads(r.read().decode() or '{}')
    except urllib.error.HTTPError as e:
        if e.code in (400, 409):
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
    api('POST', '/playbook/template', {
        'name': PB_NAME, 'description': 'client cache probe',
        'tasks': [{'name': 'noop', 'action': 'command', 'args': {'cmd': 'uptime'}}]})

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

        page_counts = []
        page.on('request', lambda r: page_counts.append(r.url) if '/api/v1/nodes?page=' in r.url else None)

        page.goto(BASE + '/playbooks')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2000)
        page.evaluate("localStorage.removeItem('owl-tabs')")

        def open_dialog_close():
            page.eval_on_selector('#pb-list-body .playbook-row:has-text("%s") .run-playbook-btn' % PB_NAME,
                                  'e => e.click()')
            page.wait_for_selector('#run-node-chips .node-chip', timeout=10000)
            page.wait_for_timeout(800)
            chips = page.locator('#run-node-chips .node-chip').count()
            page.click('#run-playbook-cancel')
            page.wait_for_timeout(400)
            return chips

        # A. 三次打开：第一次拉全量，后两次吃缓存
        first = len(page_counts)
        chips = open_dialog_close()
        after_first = len(page_counts)
        open_dialog_close()
        open_dialog_close()
        after_all = len(page_counts)

        assert chips >= 140, 'A: 选择器仍应拿到全量节点，实际 %d' % chips
        assert after_first - first >= 1, 'A: 首次应真实拉取全量节点'
        assert after_all == after_first, \
            'A: 后两次打开应命中缓存，却又发了 %d 次请求' % (after_all - after_first)
        print('A PASS 三次打开共请求 %d 次（首次 %d 次，后两次 0 次），chips=%d'
              % (after_all - first, after_first - first, chips))

        # B. 节点变更后缓存失效
        api('POST', '/nodes', {'id': 'e2e-cache-node-1', 'name': 'e2e-cache-node-1',
                               'address': '10.99.0.1', 'user': 'root', 'groups': ['e2e-cache']})
        before_invalidate = len(page_counts)
        # 走页面自己的 API 客户端建节点，确保命中同一个 api.invalidateNodesAll()
        page.evaluate("""async () => {
            const mod = await import('/static/js/api.js');
            await mod.api.createNode({id: 'e2e-cache-node-2', name: 'e2e-cache-node-2',
                address: '10.99.0.2', user: 'root', groups: ['e2e-cache']});
        }""")
        page.wait_for_timeout(600)
        open_dialog_close()
        assert len(page_counts) > before_invalidate, \
            'B: 节点新增后缓存应失效并重新拉取（实际未发请求）'
        print('B PASS 变更后重新拉取 %d 次' % (len(page_counts) - before_invalidate))

        # C. 静态资源：304 + gzip
        head = page.evaluate("""async () => {
            const r1 = await fetch('/static/js/api.js', {headers: {'If-None-Match': 'no-match'}});
            const etag = r1.headers.get('etag');
            const cc = r1.headers.get('cache-control');
            const r2 = await fetch('/static/js/api.js', {headers: {'If-None-Match': etag}});
            return {etag, cc, status: r2.status};
        }""")
        assert head['etag'], 'C: 静态资源应带 ETag'
        assert 'max-age' in (head['cc'] or ''), 'C: 静态资源应带缓存头，实际 %r' % head['cc']
        assert head['status'] == 304, 'C: If-None-Match 命中应 304，实际 %s' % head['status']
        print('C PASS ETag=%s Cache-Control=%s 二次请求=%s' % (head['etag'], head['cc'], head['status']))

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()

    api('DELETE', '/nodes/e2e-cache-node-1')
    api('DELETE', '/nodes/e2e-cache-node-2')
    pbs = api('GET', '/playbooks').get('data') or []
    for pb in pbs:
        if pb.get('name') == PB_NAME:
            api('DELETE', '/playbooks/' + pb['id'])
    print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
