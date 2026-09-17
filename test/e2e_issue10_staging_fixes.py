"""E2E: 中转站按钮可见性 + 筛选行紧凑 + 竖排表单回归

A. 表格不再横向溢出：每行删除按钮完整落在中转站卡片可视区内；清空按钮可见
B. 筛选行紧凑：分组/标签标签与控件同行（无垂直大留白），行高受限
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

        # ---------- A: 删除按钮与清空按钮可见 ----------
        rows = page.locator('#staging-file-list tbody tr')
        n = rows.count()
        assert n > 0, '前置：中转站应有文件'
        card = page.evaluate("""() => {
          const stg = [...document.querySelectorAll('.files-col-side > .card')]
            .find(c => c.textContent.includes('文件中转站'));
          const r = stg.getBoundingClientRect();
          return {left: r.left, right: r.right};
        }""")
        overflowing = 0
        for i in range(n):
            btn = rows.nth(i).locator('.staging-delete-btn')
            if btn.count() == 0:
                continue
            b = btn.bounding_box()
            if b and b['x'] + b['width'] > card['right'] + 1:
                overflowing += 1
        assert overflowing == 0, 'A: %d 行的删除按钮被挤出可视区（横向溢出未修复）' % overflowing
        clear_btn = page.locator('#staging-clear-btn')
        assert clear_btn.is_visible(), 'A: 清空按钮应可见'
        print('A PASS %d 行删除按钮全部可见，清空按钮可见' % n)

        # ---------- B: 筛选行紧凑 ----------
        m = page.evaluate("""() => {
          const rows = [...document.querySelectorAll('#files-filter-controls .filter-row')];
          return rows.map(row => {
            const label = row.querySelector(':scope > label');
            const r = row.getBoundingClientRect();
            const lr = label ? label.getBoundingClientRect() : null;
            return {rowH: Math.round(r.height), labelY: lr ? Math.round(lr.y) : null, rowY: Math.round(r.y)};
          });
        }""")
        # 传输选项行（2 列栅格）允许标签垂直居中，只约束行高紧凑
        for i, rowinfo in enumerate(m[:-1]):
            assert rowinfo['rowH'] <= 40, 'B: 筛选行 %d 高度 %d，应紧凑单行' % (i, rowinfo['rowH'])
        assert m[-1]['rowH'] <= 76, 'B: 传输选项行高度 %d，应紧凑' % m[-1]['rowH']
        gap_ok = all(abs(r['labelY'] - r['rowY']) < 12 for r in m[:-1] if r['labelY'] is not None)
        assert gap_ok, 'B: 分组/标签标签应与控件同行（无垂直大留白）: %r' % m
        print('B PASS 筛选行紧凑:', m)

        assert not errors, '不应有运行时报错: %r' % errors[:3]
        page.screenshot(path='test/e2e_issue10_final.png')
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
