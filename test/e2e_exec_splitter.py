"""E2E: 执行页可拖拉分隔条（编辑器 ⇄ 输出终端）

A. 默认高度 = 内容自然高度；矮屏下输出区不再被编辑器挤到内部滚动
B. 向下拖动分隔条 → 编辑器变高
C. 刷新后仍保持拖动后的高度（记忆在 localStorage）
D. 双击分隔条 → 复位为自然高度并清除记忆
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()


def launch(p):
    for kwargs in ({}, {'channel': 'msedge'}, {'channel': 'chrome'}):
        try:
            return p.chromium.launch(headless=True, **kwargs)
        except Exception:
            continue
    raise RuntimeError('no chromium/msedge/chrome available')


def editor_h(page):
    return page.eval_on_selector('.cmd-editor', 'e => e.offsetHeight')


def main():
    with sync_playwright() as p:
        browser = launch(p)
        # 用矮屏做验证：这里最容易暴露「编辑器固定高度挤压缩出区」的问题
        ctx = browser.new_context(viewport={'width': 1440, 'height': 620})
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN)
        page = ctx.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))
        page.goto(BASE + '/exec')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2500)
        page.evaluate("localStorage.removeItem('owl-exec-editor-h')")
        page.reload()
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2200)

        base = editor_h(page)
        assert page.locator('#exec-splitter').count() == 1, 'A: 分隔条应存在'
        print('A PASS 默认编辑器高度 = %d' % base)

        # B. 向下拖 120px
        box = page.locator('#exec-splitter').bounding_box()
        page.mouse.move(box['x'] + box['width'] / 2, box['y'] + box['height'] / 2)
        page.mouse.down()
        page.mouse.move(box['x'] + box['width'] / 2, box['y'] + 120, steps=8)
        page.mouse.up()
        page.wait_for_timeout(600)
        after = editor_h(page)
        assert after > base + 60, 'B: 向下拖动后编辑器应变高（%d -> %d）' % (base, after)
        print('B PASS 拖动后编辑器高度 %d -> %d' % (base, after))

        # C. 刷新后保持
        page.reload()
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(2200)
        kept = editor_h(page)
        assert abs(kept - after) <= 4, 'C: 刷新后应保持 %d，实际 %d' % (after, kept)
        print('C PASS 刷新后保持 %d' % kept)

        # D. 双击复位
        page.locator('#exec-splitter').dblclick()
        page.wait_for_timeout(600)
        reset = editor_h(page)
        assert abs(reset - base) <= 6, 'D: 双击应复位到 %d，实际 %d' % (base, reset)
        assert page.evaluate("localStorage.getItem('owl-exec-editor-h')") is None, 'D: 应清除记忆值'
        print('D PASS 双击复位到 %d' % reset)

        assert not errors, '页面 JS 报错: %r' % errors[:3]
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
