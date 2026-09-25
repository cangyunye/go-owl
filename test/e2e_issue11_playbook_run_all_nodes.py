"""E2E: 剧本运行弹窗的「目标节点」必须是全量节点

数据：140 个节点。e2e-a-*（50 个，组 e2e-small）按名称排序在前，
e2e-zbig-*（90 个，组 e2e-big）排序在后 —— 只取一页时 e2e-big 一个都取不到。
修复前：GET /nodes 不带 page_size 时服务端回落 20 条 → 弹窗只列 20 个节点，
        「全选」只选中 20 个，分组徽标 e2e-big 显示 0。
修复后：按 meta.total 翻页拉全量 → 140 个节点全列，全选 140，e2e-big 显示 90。
"""
import json
import sys
import urllib.request

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
PB_NAME = 'e2e-all-nodes'
SMALL, BIG = 50, 90
TOTAL = SMALL + BIG


def api(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + '/api/v1' + path, data=data, method=method, headers={
        'Authorization': 'Bearer ' + TOKEN, 'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req) as r:
            return json.loads(r.read().decode() or '{}')
    except urllib.error.HTTPError as e:
        if e.code in (400, 409):  # 已存在等幂等冲突：忽略
            return {}
        raise


def seed():
    for i in range(1, SMALL + 1):
        api('POST', '/nodes', {'id': 'e2e-a-%03d' % i, 'name': 'e2e-a-%03d' % i,
                               'address': '10.90.%d.%d' % (i // 250, i % 250), 'user': 'root',
                               'groups': ['e2e-small']})
    for i in range(1, BIG + 1):
        api('POST', '/nodes', {'id': 'e2e-zbig-%03d' % i, 'name': 'e2e-zbig-%03d' % i,
                               'address': '10.91.%d.%d' % (i // 250, i % 250), 'user': 'root',
                               'groups': ['e2e-big']})
    api('POST', '/playbook/template', {
        'name': PB_NAME, 'description': 'e2e target-node picker',
        'tasks': [{'name': 'noop', 'action': 'command', 'args': {'cmd': 'uptime'}}]})


def truth():
    """按 meta.total 翻页取全量节点，算出真实节点数/分组数。"""
    nodes, page = [], 1
    while True:
        res = api('GET', '/nodes?page=%d&page_size=100' % page)
        data = res.get('data') or []
        nodes += data
        if not data or len(nodes) >= (res.get('meta') or {}).get('total', 0):
            break
        page += 1
    counts = {}
    for n in nodes:
        for g in (n.get('groups') or []):
            counts[g] = counts.get(g, 0) + 1
    return len(nodes), counts


def launch(p):
    """优先用 Playwright 自带 chromium，未安装时退回系统 Edge/Chrome。"""
    for kwargs in ({}, {'channel': 'msedge'}, {'channel': 'chrome'}):
        try:
            return p.chromium.launch(headless=True, **kwargs)
        except Exception:
            continue
    raise RuntimeError('no chromium/msedge/chrome available: run `playwright install chromium`')


def cleanup():
    """剧本库路径是全局共享的（不在 e2e 独立 db 里），用完把种子剧本删掉。"""
    try:
        pbs = api('GET', '/playbooks').get('data') or []
        for pb in pbs:
            if pb.get('name') == PB_NAME:
                api('DELETE', '/playbooks/' + pb['id'])
                print('清理种子剧本:', PB_NAME)
    except Exception as e:
        print('清理种子剧本失败（可手动删除）:', e)
    try:
        api('DELETE', '/alert-types/OWL-CUS-E2ENODE')
    except Exception:
        pass


def main():
    seed()
    total, counts = truth()
    assert total == TOTAL, '种子数据应为 %d 个节点，实际 %d' % (TOTAL, total)
    assert counts.get('e2e-big') == BIG, 'API 真值 e2e-big 应 %d，实际 %r' % (BIG, counts.get('e2e-big'))
    print('种子/真值 OK: 节点 %d, e2e-big %d, e2e-small %d' % (total, BIG, SMALL))

    with sync_playwright() as p:
        browser = launch(p)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN
        )
        page = ctx.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.goto(BASE + '/playbooks')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(1500)

        row = page.locator('#pb-list-body .playbook-row', has_text=PB_NAME).first
        row.wait_for(state='visible', timeout=10000)
        row.locator('.run-playbook-btn').first.click()
        page.wait_for_selector('#run-playbook-modal.open', timeout=8000)
        page.wait_for_selector('#run-node-chips .node-chip', timeout=8000)
        page.wait_for_timeout(1200)

        # A. 目标节点全量，不是只列第一页
        chips = page.locator('#run-node-chips .node-chip').count()
        assert chips == total, 'A: 目标节点应列 %d 个，实际 %d 个（只取了第一页？）' % (total, chips)
        print('A PASS 目标节点 chips =', chips)

        # B. 全选选中全量节点
        page.click('#run-node-all')
        page.wait_for_timeout(400)
        sel = page.inner_text('#run-node-count')
        assert sel == str(total), 'B: 全选应选中 %d 个，实际 %r' % (total, sel)
        print('B PASS 全选计数 =', sel)

        # C. 分组徽标计数基于全量节点
        badge = page.locator('#run-group-grid .group-chip[data-group="e2e-big"] .count')
        badge.wait_for(state='visible', timeout=5000)
        big = badge.inner_text()
        assert big == str(BIG), 'C: e2e-big 徽标应 %d，实际 %r（按第一页统计？）' % (BIG, big)
        small = page.locator('#run-group-grid .group-chip[data-group="e2e-small"] .count').inner_text()
        assert small == str(SMALL), 'C: e2e-small 徽标应 %d，实际 %r' % (SMALL, small)
        print('C PASS 分组徽标 e2e-big =', big, '/ e2e-small =', small)

        # D. 搜索过滤在全量集合上工作：搜 zbig 应得 90 个可点节点
        page.fill('#run-playbook-target', 'zbig')
        page.wait_for_timeout(600)
        filtered = page.locator('#run-node-chips .node-chip').count()
        assert filtered == BIG, 'D: 搜索 zbig 应剩 %d 个，实际 %d' % (BIG, filtered)
        page.fill('#run-playbook-target', '')
        page.wait_for_timeout(400)
        print('D PASS 搜索过滤命中 =', filtered)

        page.screenshot(path='test/e2e_issue11_final.png', full_page=True)

        # E. 提交的 target_nodes 必须是全部 140 个（拦截请求，不真的下发任务）
        captured = {}
        page.click('#run-node-all')  # 重新全选（上面搜索过滤后选择集不变，这里确保全量）
        page.wait_for_timeout(300)

        def on_route(route):
            captured['body'] = json.loads(route.request.post_data or '{}')
            route.fulfill(status=202, content_type='application/json',
                          body=json.dumps({'data': {'id': 'e2e-fake-run'}}))

        page.route('**/api/v1/playbooks/*/run', on_route)
        page.click('#run-playbook-submit')
        page.wait_for_timeout(800)
        targets = captured.get('body', {}).get('target_nodes') or []
        assert len(targets) == total, 'E: 提交的 target_nodes 应 %d 个，实际 %d 个' % (total, len(targets))
        print('E PASS 提交 target_nodes =', len(targets))

        # F. 告警调试弹窗的节点下拉（同款缺陷：单次请求被钳到 100 条）
        page.unroute('**/api/v1/playbooks/*/run')
        api('POST', '/alert-types', {
            'id': 'OWL-CUS-E2ENODE', 'name': 'e2e-node-picker', 'category': 'custom',
            'default_severity': 'warn',
            'default_params': {'metric': 'custom_e2e_node', 'op': '>', 'value': 1, 'duration': 1},
            'enabled': True, 'notifiable': False, 'builtin': False,
            'check_cmd': 'uptime', 'check_mode': 'exit_code', 'check_pattern': '',
            'scope_nodes': '', 'scope_groups': ''})
        page.goto(BASE + '/alerts')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2000)
        page.click('#toggle-config')
        page.wait_for_timeout(1500)
        dbg = page.locator('.at-debug[data-at-debug="OWL-CUS-E2ENODE"]').first
        dbg.wait_for(state='visible', timeout=10000)
        dbg.click()
        page.wait_for_selector('#dbg-node', timeout=8000)
        page.wait_for_timeout(1200)
        opts = page.locator('#dbg-node option').count()
        assert opts == total, 'F: 告警调试节点下拉应 %d 个，实际 %d 个' % (total, opts)
        print('F PASS 告警调试节点下拉 =', opts)

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()
        cleanup()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        cleanup()
        print('FAIL:', e)
        sys.exit(1)
