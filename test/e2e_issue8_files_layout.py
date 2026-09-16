"""E2E: /files 页 tab 稳定 + 分页 + 双栏布局

前提：服务端已有 21+ 条传输记录（脚本外经 API 造数）。
A. tab 稳定：点"任务详情"后跨多个 5s 轮询周期 active 不变；单击"传输记录"立即切回
B. 分页：分页条显示"共 N 条 · 第 1/2 页"，翻页后可见更早记录，页码状态正确
C. 双栏布局：中转站卡片与传输表单同排（顶端 y 坐标接近），且不随记录数变化
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

        # ---------- C: 双栏布局 ----------
        form_box = page.locator('.files-grid .card').first.bounding_box()
        staging_top = page.evaluate("""() => {
          const cards = [...document.querySelectorAll('.exec-main .files-grid > .card')];
          const stg = cards.find(c => c.textContent.includes('文件中转站'));
          return stg ? stg.getBoundingClientRect() : null;
        }""")
        assert staging_top, 'C: 中转站卡片应在 files-grid 内'
        assert abs(staging_top['y'] - form_box['y']) < 40, 'C: 中转站应与表单同排: form y=%s stg y=%s' % (form_box['y'], staging_top['y'])
        print('C PASS 中转站与表单并排 (y: %s vs %s)' % (round(form_box['y']), round(staging_top['y'])))

        # ---------- A: tab 稳定 ----------
        page.locator('#transfer-tabs .seg button[data-tab="tasks"]').click()
        page.wait_for_timeout(500)
        for _ in range(3):  # 跨 2-3 个轮询周期
            page.wait_for_timeout(5500)
            active = page.locator('#transfer-tabs .seg button.active').get_attribute('data-tab')
            assert active == 'tasks', 'A: 跨轮询周期 tab 应回到 tasks，实际 %r' % active
        print('A PASS 任务详情 tab 跨轮询周期稳定')

        # 单击"传输记录"应立即切换
        page.locator('#transfer-tabs .seg button[data-tab="list"]').click()
        page.wait_for_timeout(400)
        active = page.locator('#transfer-tabs .seg button.active').get_attribute('data-tab')
        assert active == 'list', 'A: 单击传输记录应立即切换: %r' % active
        print('A PASS 单击切换正常')

        # ---------- B: 分页 ----------
        info = page.inner_text('#transfer-pager .page-info')
        assert '第 1/2 页' in info, 'B: 应显示第 1/2 页: %r' % info
        total = int(info.split('共 ')[1].split(' 条')[0])
        assert total >= 21, 'B: 前置数据不足（需 >=21 条），实际 %d' % total
        page.locator('#transfer-pager .page-btn', has_text='2').first.click()
        page.wait_for_timeout(1200)
        info2 = page.inner_text('#transfer-pager .page-info')
        assert '第 2/2 页' in info2, 'B: 翻页后应在第 2 页: %r' % info2
        rows = page.locator('#transfer-list .task-item').count()
        assert rows == total - 20, 'B: 第 2 页应为 %d 行，实际 %d' % (total - 20, rows)
        print('B PASS 分页: %s，第二页 %d 行' % (info2, rows))

        # 中转站位置在翻页后不变化
        staging_top2 = page.evaluate("""() => {
          const cards = [...document.querySelectorAll('.exec-main .files-grid > .card')];
          const stg = cards.find(c => c.textContent.includes('文件中转站'));
          return stg.getBoundingClientRect().y;
        }""")
        assert abs(staging_top2 - staging_top['y']) < 4, 'C: 翻页后中转站位置不应变化'
        print('C PASS 翻页后中转站位置稳定')

        assert not errors, '不应有运行时报错: %r' % errors[:3]
        page.screenshot(path='test/e2e_issue8_final.png')
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
