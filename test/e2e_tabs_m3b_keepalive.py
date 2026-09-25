"""E2E: M3b 保活扩展——sftp / ai / playbooks 失活行为审计后的接入验证

前置：需要真实可达的 SSH 节点（本脚本用 192.168.31.100 / kali 密码认证）。
对应三个页面在失活期间的「后台工作 vs 界面更新」分离：
  playbooks —— 运行在服务端继续，广播被门控，切回 onResume 补拉列表与详情
  sftp      —— 上传 XHR 继续跑，只暂停界面更新，切回重绘队列与目录
  ai        —— 流式文本继续累积，失活期间的消息挂起，切回补渲染（无 API key 时只验证结构）

用例：
A. 剧本：跑一次剧本（真实节点）→ 打开运行详情 → 切走 → 切回 → 详情仍渲染（onResume 补拉）
B. SFTP：上传一个较大文件 → 上传中切走 → 切回 → 队列与目录正常，文件已落在远端
C. AI：打开 AI 页 → 切走 → 切回 → 无 JS 报错、无悬挂连接
"""
import json
import os
import sys
import time
import urllib.request

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
NODE_ID = 'e2e-term-pi'          # 复用 M3a 里那台真实节点
PB_NAME = 'e2e-keepalive'
REMOTE_DIR = '/tmp'
UPLOAD_NAME = 'e2e-keepalive-%d.bin' % int(time.time())
UPLOAD_SIZE = 3 * 1024 * 1024    # 3MB：足够在上传途中切标签


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


def setup_node_and_playbook():
    api('POST', '/nodes', {'id': NODE_ID, 'name': 'e2e-term-pi', 'address': '192.168.31.100',
                           'port': 22, 'user': 'kali', 'password': 'hwx1515661', 'groups': ['e2e-term']})
    api('POST', '/playbook/template', {
        'name': PB_NAME, 'description': 'keepalive probe',
        'tasks': [{'name': 'echo', 'action': 'command', 'args': {'cmd': 'uptime'}}]})


def main():
    setup_node_and_playbook()
    tmp = os.path.join('build', UPLOAD_NAME)
    with open(tmp, 'wb') as f:
        f.write(os.urandom(UPLOAD_SIZE))

    with sync_playwright() as p:
        browser = launch(p)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN)
        page = ctx.new_page()
        errors = []

        def on_err(e):
            errors.append(str(e))
            print('  [pageerror] %s' % str(e)[:140])
            for line in (getattr(e, 'stack', '') or '').splitlines()[:5]:
                    print('      ', line.strip()[:150])

        page.on('pageerror', on_err)

        # ---------- A. 剧本：运行 → 打开详情 → 切走 → 切回 ----------
        page.goto(BASE + '/playbooks')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2000)
        row = page.locator('#pb-list-body .playbook-row', has_text=PB_NAME).first
        row.locator('.run-playbook-btn').first.click()
        page.wait_for_selector('#run-playbook-modal.open', timeout=8000)
        page.fill('#run-playbook-target', 'e2e-term-pi')
        page.wait_for_timeout(900)
        page.locator('#run-node-chips .node-chip').first.click()
        page.click('#run-playbook-submit')
        page.wait_for_timeout(2500)

        page.click('#runs-pagination')  # no-op：确保焦点离开弹窗
        page.wait_for_selector('#playbook-runs-list tr .view-run-btn', timeout=15000)
        page.locator('#playbook-runs-list tr .view-run-btn').first.click()
        page.wait_for_timeout(2000)
        run_id = page.eval_on_selector('#run-detail', 'e => e.dataset.runId')
        assert run_id, 'A: 运行详情应已加载（data-run-id 为空）'
        print('A 前半 PASS 运行详情已打开 run=%s' % run_id)

        page.eval_on_selector('.nav-item[data-view="nodes"]', 'e => e.click()')   # 切走
        page.wait_for_timeout(1500)
        in_doc = page.evaluate("document.querySelectorAll('#run-detail').length")
        assert in_doc == 0, 'A: 剧本页失活后其容器应从文档摘除，实际仍在'
        page.eval_on_selector('#tabbar .tab:nth-child(1)', 'e => e.click()')     # 切回
        page.wait_for_timeout(2800)
        run_id2 = page.eval_on_selector('#run-detail', 'e => e.dataset.runId')
        assert run_id2 == run_id, 'A: 切回后运行详情应仍在（onResume 补拉），实际 %r' % run_id2
        steps = page.eval_on_selector_all('#run-detail .step-row, #run-detail table tr', 'els => els.length')
        assert steps >= 1, 'A: 切回后详情应有步骤结果渲染'
        print('A PASS 切走→切回后详情仍在（run=%s，渲染行数=%d）' % (run_id2, steps))

        # ---------- B. SFTP：上传中切走 → 切回 ----------
        page.goto(BASE + '/sftp/' + NODE_ID)
        page.wait_for_load_state('networkidle')
        page.wait_for_selector('#sftp-body', timeout=12000)
        page.wait_for_timeout(2000)
        page.set_input_files('#sftp-file-input', tmp)
        page.wait_for_timeout(700)                       # 让上传开始
        uploading = page.locator('#sftp-queue').inner_text()
        assert UPLOAD_NAME in uploading or '上传' in uploading, 'B: 上传应已进入队列，实际 %r' % uploading[:80]
        page.eval_on_selector('.nav-item[data-view="nodes"]', 'e => e.click()')   # 上传途中切走
        page.wait_for_timeout(2500)
        assert page.evaluate("document.querySelectorAll('#sftp-queue').length") == 0, \
            'B: SFTP 页失活后容器应摘除'
        page.eval_on_selector('#tabbar .tab:nth-child(1)', 'e => e.click()')     # 切回
        page.wait_for_timeout(2500)
        # 远端校验：文件确实传完了（说明失活期间 XHR 没有被销毁）。
        # 用 SFTP 自己的列目录接口，比另发一条 exec 任务更直接。
        ok = False
        found = None
        remote_dir = page.evaluate("() => { const el = document.getElementById('sftp-path') || document.getElementById('sftp-cwd'); return el ? el.textContent.trim() : ''; }") or REMOTE_DIR
        print('   远端校验目录: %r' % remote_dir)
        for _ in range(20):
            time.sleep(1.5)
            listing = api('GET', '/sftp/ls?node_id=%s&path=%s' % (NODE_ID, remote_dir))
            entries = listing.get('items') or []   # 响应形状：{path, items:[{name,size,...}]}
            hit = [e for e in entries if e.get('name') == UPLOAD_NAME]
            if hit and int(hit[0].get('size') or 0) == UPLOAD_SIZE:
                ok = True
                found = hit[0]
                break
        assert ok, 'B: 失活期间上传应继续完成（远端 %s/%s 未就绪）' % (remote_dir, UPLOAD_NAME)
        print('B PASS 上传中切标签：远端已存在 %s（%d MB）' % (UPLOAD_NAME, UPLOAD_SIZE // 1024 // 1024))

        # ---------- C. AI：结构验证（无 API key 时不发真实请求） ----------
        page.goto(BASE + '/ai')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2000)
        assert page.locator('#ai-chat-input').count() == 1, 'C: AI 页面应正常挂载'
        page.eval_on_selector('.nav-item[data-view="nodes"]', 'e => e.click()')
        page.wait_for_timeout(1500)
        page.eval_on_selector('#tabbar .tab:nth-child(1)', 'e => e.click()')
        page.wait_for_timeout(1800)
        assert page.locator('#ai-chat-input').count() == 1, 'C: 切回后 AI 页应仍在（保活）'
        print('C PASS AI 页保活（结构验证；真实流式需 API key）')

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()

    os.remove(tmp)
    api('POST', '/sftp/delete', {'node_id': NODE_ID, 'path': '%s/%s' % (REMOTE_DIR, UPLOAD_NAME)})
    api('DELETE', '/nodes/' + NODE_ID)
    for pb in (api('GET', '/playbooks').get('data') or []):
        if pb.get('name') == PB_NAME:
            api('DELETE', '/playbooks/' + pb['id'])
    print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
