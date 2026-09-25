"""E2E: M3 会话类保活（方案 A：同文档保活）——以终端为验证对象

前置：需要一个**真实可达**的 SSH 节点（本脚本用树莓派 192.168.31.100 / kali 密码认证，
      连不上会明确报错）。终端是保活的旗舰场景：切标签不能掐断 shell。

覆盖：
A. 终端页连上后，往 shell 打一条标记命令，输出出现在 xterm 里
B. 切到别的标签：终端容器被摘出文档（不是 display:none），但 WS 连接仍在
C. 切回终端标签：同一个 DOM 实例（探针属性还在）、滚动缓冲/输出还在、WS 未重连
D. 切回后 shell 仍可用（再打一条命令，输出正常），且没有出现第二个 xterm 实例
"""
import json
import sys
import time
import urllib.request

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()
NODE_ID = 'e2e-term-pi'
MARK = 'MARK-%d' % int(time.time())


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


INSTRUMENT = r"""
window.__sockets = 0;
(function () {
  const WS = window.WebSocket;
  function CountingWS(...a) {
    const ws = new WS(...a); window.__sockets++;
    let closed = false;
    const mark = () => { if (!closed) { closed = true; window.__sockets--; } };
    const close = ws.close.bind(ws);
    ws.close = function (...b) { mark(); return close(...b); };
    ws.addEventListener('close', mark);
    return ws;
  }
  CountingWS.CONNECTING = WS.CONNECTING; CountingWS.OPEN = WS.OPEN;
  CountingWS.CLOSING = WS.CLOSING; CountingWS.CLOSED = WS.CLOSED;
  CountingWS.prototype = WS.prototype;
  window.WebSocket = CountingWS;
})();
"""


def launch(p):
    for kwargs in ({}, {'channel': 'msedge'}, {'channel': 'chrome'}):
        try:
            return p.chromium.launch(headless=True, **kwargs)
        except Exception:
            continue
    raise RuntimeError('no chromium/msedge/chrome available')


def frame_text(page):
    return page.evaluate("document.querySelector('.xterm-rows') ? document.querySelector('.xterm-rows').innerText : ''")


def main():
    api('POST', '/nodes', {'id': NODE_ID, 'name': 'e2e-term-pi', 'address': '192.168.31.100',
                           'port': 22, 'user': 'kali', 'password': 'hwx1515661', 'groups': ['e2e-term']})

    with sync_playwright() as p:
        browser = launch(p)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
        ctx.add_init_script(INSTRUMENT)
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN)
        page = ctx.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))

        # A. 打开终端并确认连上
        page.goto(BASE + '/terminal/' + NODE_ID)
        page.wait_for_load_state('networkidle')
        page.wait_for_selector('.xterm', timeout=10000)
        page.wait_for_timeout(3000)
        status = page.eval_on_selector('#term-status', 'e => e.textContent.trim()') if page.locator('#term-status').count() else ''
        assert '已连接' in status or '已断开' not in status, \
            'A: 终端应连上真实节点（%s），实际状态 %r——检查测试机 192.168.31.100 是否可达' % (NODE_ID, status)

        page.click('.xterm')
        page.keyboard.type('echo %s\n' % MARK, delay=20)
        page.wait_for_timeout(2000)
        text = frame_text(page)
        assert MARK in text, 'A: shell 回显应出现在 xterm 中，实际未找到 %s' % MARK
        print('A PASS 终端已连接并回显 %s' % MARK)

        sockets_before = page.evaluate('window.__sockets')
        assert sockets_before == 1, 'A: 应只有 1 条终端 WS，实际 %d' % sockets_before

        # 给 DOM 打探针：切回后仍在，说明没有重新挂载
        page.evaluate("document.querySelector('.xterm').dataset.keepAliveProbe = 'probe-1'")

        # B. 切到别的标签
        page.eval_on_selector('.nav-item[data-view="nodes"]', 'e => e.click()')
        page.wait_for_timeout(2200)
        in_doc = page.evaluate("document.querySelectorAll('.xterm').length")
        assert in_doc == 0, 'B: 失活的终端容器应从文档摘除，实际仍在（%d 个）' % in_doc
        assert page.evaluate('window.__sockets') == 1, \
            'B: 切走后终端 WS 不应断开，实际 %d' % page.evaluate('window.__sockets')
        print('B PASS 切走后：容器已摘除、WS 仍保持 1 条')

        # C. 切回终端标签
        page.eval_on_selector('#tabbar .tab:nth-child(1)', 'e => e.click()')
        page.wait_for_timeout(2200)
        probe = page.evaluate("document.querySelector('.xterm') ? document.querySelector('.xterm').dataset.keepAliveProbe : null")
        assert probe == 'probe-1', 'C: 切回应复用同一 DOM 实例（探针丢失说明被重新挂载），实际 %r' % probe
        assert page.evaluate("document.querySelectorAll('.xterm').length") == 1, 'C: 不应出现第二个 xterm 实例'
        assert page.evaluate('window.__sockets') == 1, \
            'C: 切回不应重连 WS，实际 %d' % page.evaluate('window.__sockets')
        text = frame_text(page)
        assert MARK in text, 'C: 切回后滚动缓冲应还在（未找到 %s）' % MARK
        print('C PASS 切回：同一 DOM、缓冲保留、WS 未重连')

        # D. shell 仍可用
        page.click('.xterm')
        page.keyboard.type('echo AFTER-%s\n' % MARK, delay=20)
        page.wait_for_timeout(2000)
        assert ('AFTER-%s' % MARK) in frame_text(page), 'D: 切回后 shell 应仍可用'
        print('D PASS 切回后 shell 仍可用')

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
