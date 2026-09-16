"""E2E: 中转站工作台（竖排表单/名称优先+图标/批量删除）

说明：清空按钮仅做存在性与显隐校验，不点击（避免删除真实中转站文件）。
"""
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()


def main():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context(viewport={'width': 1440, 'height': 900})
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN
        )
        page = ctx.new_page()
        errors = []
        page.on('pageerror', lambda e: errors.append(str(e)))

        page.goto(BASE + '/files')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(1500)

        # ---------- 竖排表单 + 无大留白 ----------
        src_box = page.locator('#src-path').bounding_box()
        dst_box = page.locator('#dst-path').bounding_box()
        assert abs(src_box['x'] - dst_box['x']) < 4, '竖排：两个输入框应左对齐'
        assert dst_box['y'] > src_box['y'], '竖排：节点路径应在本地路径下方'
        form_h = page.evaluate("""() => {
          const cards = [...document.querySelectorAll('.files-col-main > .card')];
          const form = cards.find(c => c.textContent.includes('文件传输'));
          return form ? form.getBoundingClientRect().height : 9999;
        }""")
        assert form_h < 320, '表单卡不应有大片留白（高度 %s）' % form_h
        print('C PASS 竖排表单，表单卡高度 %d' % round(form_h))

        # ---------- 名称优先 + 图标 + 悬浮全路径 ----------
        page.fill('#staging-search', 'e2e-temp-cleanup')
        page.wait_for_timeout(800)
        cell = page.locator('#staging-file-list tbody .stg-name').first
        name_text = cell.inner_text()
        assert 'e2e-temp-cleanup.txt' in name_text, '名称列应显示完整文件名: %r' % name_text
        assert cell.get_attribute('title') == '/Users/vigil/.owl/staging/e2e-temp-cleanup.txt', \
            '悬浮 title 应为完整路径: %r' % cell.get_attribute('title')
        assert cell.locator('svg.stg-icon').count() == 1, '名称左侧应有类型图标'
        assert 'path d="M6 8.5h4' in cell.locator('svg.stg-icon').inner_html(), 'txt 应归类为文本图标'
        print('B PASS 名称优先 + 图标 + 悬浮全路径')

        # ---------- 批量删除选中 ----------
        page.locator('#staging-multi-btn').click()
        page.wait_for_timeout(300)
        page.locator('#staging-file-list .staging-checkbox').first.check()
        page.wait_for_timeout(300)
        del_btn = page.locator('#staging-delete-selected-btn')
        assert del_btn.is_visible(), '多选选中后应出现删除选中按钮'
        assert '1' in del_btn.inner_text(), '按钮应带计数: %r' % del_btn.inner_text()
        page.once('dialog', lambda d: d.accept())
        del_btn.click()
        page.wait_for_timeout(1500)
        page.fill('#staging-search', 'e2e-temp-cleanup')
        page.wait_for_timeout(800)
        body = page.inner_text('#staging-file-list')
        assert 'e2e-temp-cleanup' not in body, '删除选中后文件应消失: %r' % body
        print('B PASS 批量删除选中生效')

        # ---------- 清空按钮存在且可见（不点击） ----------
        page.fill('#staging-search', '')
        page.wait_for_timeout(800)
        clear_btn = page.locator('#staging-clear-btn')
        assert clear_btn.is_visible(), 'admin 且有文件时清空按钮应可见'
        print('A PASS 清空按钮存在且可见（不点击）')

        assert not errors, '不应有运行时报错: %r' % errors[:3]
        page.screenshot(path='test/e2e_issue9_final.png')
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
