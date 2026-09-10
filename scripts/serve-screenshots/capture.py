#!/usr/bin/env python3
"""owl-serve Web 控制台截图采集（docs/serve-web-ui.md 配套）。

前置条件：
  1. owl-serve 已在本机运行（默认 http://localhost:8080），
     且已构建包含最新前端资源的二进制（task build-serve）。
  2. 已准备演示数据（见 seed_demo_data.py）。

用法：
  python3 scripts/serve-screenshots/capture.py --password <admin密码> \
      [--base-url http://localhost:8080] [--out docs/images/serve]

输出：26 张 PNG（01-login ... 26-theme-warm），文件名与 docs/serve-web-ui.md 引用一致。
"""
import argparse
import os
import sys
import time
from playwright.sync_api import sync_playwright

SHOTS_EXPECTED = 26


def close_overlays(page):
    for sel in ["#detail-close-btn", "#detail-cancel-btn", "#run-playbook-cancel",
                "#settings-cancel", "#add-setting-cancel", "#detail-close",
                ".modal-close"]:
        try:
            loc = page.locator(sel).first
            if loc.is_visible(timeout=400):
                loc.click()
                page.wait_for_timeout(300)
        except Exception:
            pass
    page.evaluate("""() => document.querySelectorAll('.modal-overlay, .overlay, .dialog-overlay')
                      .forEach(e => e.remove())""")
    page.wait_for_timeout(200)


def nav(page, view):
    close_overlays(page)
    page.click(f'.nav-item[data-view="{view}"]')
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(800)


def run(args):
    out = args.out
    os.makedirs(out, exist_ok=True)
    taken = []

    def shot(page, name):
        page.wait_for_timeout(500)
        page.screenshot(path=f"{out}/{name}.png")
        taken.append(name)
        print(f"[ok] {name}", flush=True)

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context(viewport={"width": 1440, "height": 900},
                                  device_scale_factor=2, locale="zh-CN")
        page = ctx.new_page()
        page.on("dialog", lambda d: d.accept())  # 吞掉 confirm() 弹窗

        # 01 登录页（未登录状态）
        page.goto(f"{args.base_url}/login")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(600)
        shot(page, "01-login")

        # 登录
        page.fill("#username", args.user)
        page.fill("#password", args.password)
        page.click("#login-btn")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(1500)

        # 02 仪表盘（默认深空主题）
        shot(page, "02-dashboard")

        # 03-06 节点管理
        nav(page, "nodes")
        page.wait_for_timeout(700)
        shot(page, "03-nodes-list")
        page.fill("#search-input", "web")
        page.wait_for_timeout(600)
        shot(page, "04-nodes-search")
        page.fill("#search-input", "")
        page.wait_for_timeout(500)
        page.click("#add-node-btn")
        page.wait_for_timeout(600)
        shot(page, "05-node-add-modal")
        close_overlays(page)
        boxes = page.locator("tbody input[type='checkbox']")
        for i in range(min(3, boxes.count())):
            boxes.nth(i).check()
            page.wait_for_timeout(150)
        page.wait_for_timeout(500)
        shot(page, "06-nodes-batch")

        # 07 节点详情（直连路由）
        page.goto(f"{args.base_url}/nodes/node-01")
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(900)
        shot(page, "07-node-detail")

        # 08-10 命令执行
        nav(page, "exec")
        page.fill("#cmd-input", "uptime && df -h")
        page.wait_for_timeout(400)
        shot(page, "08-exec-command")
        page.click("#tab-script")
        page.wait_for_timeout(400)
        shot(page, "09-exec-script")
        page.click("#tab-command")
        page.fill("#cmd-input", "uptime")
        try:  # 选中 web 分组，缩小执行范围
            page.locator(".group-chip, .group-item, [data-group]").filter(
                has_text="web").first.click(timeout=2500)
            page.wait_for_timeout(300)
        except Exception:
            pass
        page.click("#exec-btn")
        page.wait_for_timeout(2500)
        shot(page, "10-exec-running")

        # 11-12 任务历史与详情
        nav(page, "history")
        page.wait_for_timeout(700)
        shot(page, "11-history")
        try:
            # 优先选取已终态（失败）的记录：刚触发的 exec 会排在最前且尚无明细
            items = page.locator(".history-item[data-task]")
            done = items.filter(has_text="失败")
            (done.first if done.count() else items.first).click(timeout=3000)
            page.wait_for_timeout(1500)
            shot(page, "12-history-detail")
        except Exception as e:
            print(f"[warn] history-detail: {e}", flush=True)
        close_overlays(page)

        # 13-15 剧本管理
        nav(page, "playbooks")
        page.wait_for_timeout(1000)
        shot(page, "13-playbooks")
        try:
            rows = page.locator("tr.playbook-row")
            row = rows.filter(has_text="Docs Demo")
            (row.first if row.count() else rows.first).click(timeout=3000)
            page.wait_for_timeout(1200)
            shot(page, "14-playbook-detail")
            page.click("#detail-run-btn", timeout=2000)
            page.wait_for_timeout(900)
            shot(page, "15-playbook-run-modal")
        except Exception as e:
            print(f"[warn] playbook-detail: {e}", flush=True)
        close_overlays(page)

        # 16-17 文件传输
        nav(page, "files")
        page.wait_for_timeout(900)
        shot(page, "16-files-upload")
        try:
            page.locator("button:has-text('任务详情')").first.click(timeout=2000)
            page.wait_for_timeout(500)
            shot(page, "17-files-records")
        except Exception as e:
            print(f"[warn] files-records: {e}", flush=True)

        # 18 AI 助手（需已在系统设置配置 LLM，否则仅界面框架）
        nav(page, "ai")
        page.fill("#ai-chat-input", "查看在线节点")
        page.wait_for_timeout(600)
        shot(page, "18-ai-chat")

        # 19-20 告警中心
        nav(page, "alerts")
        page.wait_for_timeout(900)
        shot(page, "19-alerts")
        try:
            page.click("#toggle-config")
            page.wait_for_timeout(700)
            shot(page, "20-alerts-config")
        except Exception as e:
            print(f"[warn] alerts-config: {e}", flush=True)

        # 21-22 系统设置（admin）
        nav(page, "settings")
        page.wait_for_timeout(900)
        shot(page, "21-settings")
        try:
            page.locator(".edit-setting-btn").first.click(timeout=2000)
            page.wait_for_timeout(500)
            shot(page, "22-settings-edit")
        except Exception as e:
            print(f"[warn] settings-edit: {e}", flush=True)
        close_overlays(page)

        # 23-24 用户管理（admin）
        nav(page, "users")
        page.wait_for_timeout(700)
        shot(page, "23-users")
        try:
            page.click("#add-user-btn")
            page.wait_for_timeout(500)
            shot(page, "24-users-add-modal")
        except Exception as e:
            print(f"[warn] users-add-modal: {e}", flush=True)
        close_overlays(page)

        # 25-26 主题（回到仪表盘切换）
        nav(page, "dashboard")
        page.click('.theme-btn[data-theme-val="light-sky"]')
        page.wait_for_timeout(600)
        shot(page, "25-theme-light")
        page.click('.theme-btn[data-theme-val="dark-warm"]')
        page.wait_for_timeout(600)
        shot(page, "26-theme-warm")

        browser.close()

    # 校验产物
    missing = [n for n in taken
               if not os.path.isfile(f"{out}/{n}.png")
               or os.path.getsize(f"{out}/{n}.png") < 10_000]
    print(f"\nDONE: {len(taken)}/{SHOTS_EXPECTED} shots -> {out}")
    if missing:
        print("MISSING/EMPTY:", missing)
        return 1
    return 0


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--base-url", default="http://localhost:8080")
    ap.add_argument("--user", default="admin")
    ap.add_argument("--password", required=True, help="admin 密码")
    ap.add_argument("--out", default="docs/images/serve")
    args = ap.parse_args()

    start = time.time()
    rc = run(args)
    print(f"elapsed: {time.time() - start:.1f}s")
    sys.exit(rc)


if __name__ == "__main__":
    main()
