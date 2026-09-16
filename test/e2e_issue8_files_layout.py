"""E2E: /files 页双栏布局 + tab 稳定 + 分页

布局：左列 = 传输表单 + 传输记录（纵向对齐）；右列 = 筛选条件（上）+ 文件中转站（下）。
A. tab 稳定：点"任务详情"后跨多个 5s 轮询周期 active 不变；单击"传输记录"立即切回
B. 分页：分页条显示"共 N 条 · 第 1/2 页"，翻页后可见更早记录，页码状态正确
C. 双栏布局：右列在左列右侧，筛选条件在中转站上方，传输记录在表单下方
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

        # ---------- C: 双栏布局几何 ----------
        geo = page.evaluate("""() => {
          const r = el => el.getBoundingClientRect();
          const side = document.querySelector('.files-col-side');
          const main = document.querySelector('.files-col-main');
          if (!side || !main) return null;
          const sideCards = [...side.querySelectorAll(':scope > .card')];
          const mainCards = [...main.querySelectorAll(':scope > .card')];
          const filters = sideCards.find(c => c.textContent.includes('筛选条件'));
          const stg = sideCards.find(c => c.textContent.includes('文件中转站'));
          const form = mainCards.find(c => c.textContent.includes('文件传输'));
          const rec = mainCards.find(c => c.textContent.includes('传输记录'));
          return {
            mainX: r(main).x, sideX: r(side).x, sideW: r(side).width,
            filtersY: filters ? r(filters).y : null,
            stgY: stg ? r(stg).y + (document.querySelector(".view-container")?.scrollTop || 0) : null,
            formY: form ? r(form).y : null,
            recY: rec ? r(rec).y : null,
            pathCol: !!side.querySelector('.staging-table .stg-path'),
          };
        }""")
        assert geo, 'C: 双栏结构缺失'
        assert geo['sideX'] > geo['mainX'], 'C: 右列应在左列右侧'
        assert geo['filtersY'] is not None and geo['stgY'] is not None
        assert geo['stgY'] > geo['filtersY'], 'C: 中转站应在筛选条件下方'
        assert geo['formY'] is not None and geo['recY'] is not None
        assert geo['recY'] > geo['formY'], 'C: 传输记录应在表单下方（纵向对齐）'
        assert geo['pathCol'], 'C: 中转站表格应有路径列'
        assert geo['sideW'] >= 400, 'C: 右列宽度应 >= 400，实际 %s' % geo['sideW']
        print('C PASS 双栏布局:', {k: (round(v) if isinstance(v, (int, float)) else v) for k, v in geo.items() if v is not None})

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

        # 中转站位置在翻页后不变化（相对滚动容器 .view-container 的文档坐标：
        # 页面滚动发生在 .view-container 上而非 window，点击翻页按钮会触发
        # 容器滚动，必须用 scrollTop 折算才能得到稳定的文档坐标）
        geo2 = page.evaluate("""() => {
          const stg = [...document.querySelectorAll('.files-col-side > .card')]
            .find(c => c.textContent.includes('文件中转站'));
          const scroller = document.querySelector('.view-container');
          return stg.getBoundingClientRect().y + (scroller ? scroller.scrollTop : 0);
        }""")
        assert abs(geo2 - geo['stgY']) < 4, 'C: 翻页后中转站位置不应变化'
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
