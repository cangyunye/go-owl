"""E2E: 文件传输页目标节点防护（问题1）

场景：
A. 未选择任何节点/分组/标签 → 点击上传 → confirm 提示全量传输（全部 51 个节点），取消后不产生记录
B. 全选 51 个节点 → 点击上传 → confirm 提示超过 50 个，取消后不产生记录
C. 仅选 1 个节点 → 点击上传 → 不出确认框，直接提交（alert 已提交）
"""
import json
import sys

from playwright.sync_api import sync_playwright

BASE = 'http://127.0.0.1:18099'
TOKEN = open('test/e2e-token.txt').read().strip()

dialogs = []


def new_dialogs(idx):
    return dialogs[idx:]


def main():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context()
        ctx.add_init_script(
            "window.localStorage.setItem('token', '%s');"
            "window.localStorage.setItem('user', JSON.stringify({id:'admin', username:'admin', role:'admin'}));"
            % TOKEN
        )
        page = ctx.new_page()
        page.on('dialog', lambda d: (dialogs.append({'type': d.type, 'message': d.message}), d.dismiss()))

        page.goto(BASE + '/files')
        page.wait_for_load_state('networkidle')
        page.wait_for_timeout(1500)

        # ---------- Case A: 未选择任何目标 ----------
        mark = len(dialogs)
        page.click('#upload-btn')
        page.wait_for_timeout(1500)
        got = new_dialogs(mark)
        assert got, 'A: 未出现任何对话框'
        assert got[0]['type'] == 'confirm', 'A: 应为 confirm，得到 %s' % got[0]['type']
        assert '未选择任何分组/标签' in got[0]['message'], 'A: 文案缺少全量警告: %r' % got[0]['message']
        assert '全部 51 个节点' in got[0]['message'], 'A: 未统计到 51 个节点: %r' % got[0]['message']
        print('A PASS confirm:', got[0]['message'].replace('\n', ' | '))
        list_text = page.inner_text('#transfer-list')
        assert '暂无传输记录' in list_text, 'A: 取消后不应产生传输记录: %r' % list_text
        print('A PASS 取消后无记录')

        # ---------- Case B: 全选 51 个节点 ----------
        page.click('#select-all-btn')
        page.wait_for_timeout(2000)
        count_txt = page.inner_text('#selected-count')
        assert '51' in count_txt, 'B: 全选后应已选 51 个节点: %r' % count_txt
        mark = len(dialogs)
        page.click('#upload-btn')
        page.wait_for_timeout(1500)
        got = new_dialogs(mark)
        assert got, 'B: 未出现任何对话框'
        assert got[0]['type'] == 'confirm', 'B: 应为 confirm，得到 %s' % got[0]['type']
        assert '超过 50 个' in got[0]['message'] and '51 个节点' in got[0]['message'], 'B: 文案缺少超限警告: %r' % got[0]['message']
        print('B PASS confirm:', got[0]['message'].replace('\n', ' | '))
        list_text = page.inner_text('#transfer-list')
        assert '暂无传输记录' in list_text, 'B: 取消后不应产生传输记录: %r' % list_text
        print('B PASS 取消后无记录')

        # ---------- Case C: 仅选 1 个节点，直接放行 ----------
        page.click('#clear-selection-btn')
        page.wait_for_timeout(300)
        page.locator('#panel-node-list .node-chip').first.click()
        page.wait_for_timeout(300)
        mark = len(dialogs)
        page.click('#upload-btn')
        page.wait_for_timeout(2000)
        got = new_dialogs(mark)
        assert got, 'C: 应出现提交提示 alert'
        assert got[0]['type'] == 'alert', 'C: 单节点不应出现 confirm，得到 %s: %r' % (got[0]['type'], got[0]['message'])
        assert '已提交' in got[0]['message'] and '1 个节点' in got[0]['message'], 'C: 提交提示异常: %r' % got[0]['message']
        print('C PASS alert:', got[0]['message'])
        page.wait_for_timeout(1200)
        list_text = page.inner_text('#transfer-list')
        assert '暂无传输记录' not in list_text, 'C: 提交后应出现传输记录: %r' % list_text
        print('C PASS 提交后产生记录')

        page.screenshot(path='test/e2e_issue1_final.png', full_page=True)
        browser.close()
        print('ALL PASS')


if __name__ == '__main__':
    try:
        main()
    except AssertionError as e:
        print('FAIL:', e)
        sys.exit(1)
