#!/usr/bin/env python3
"""owl-serve 截图演示数据准备（幂等）。

为 docs/serve-web-ui.md 的截图准备内容：
  - 种子节点（POST /api/v1/nodes/seed，50 个模拟节点）
  - 演示用户 bob(editor)/carol(viewer)（已存在则跳过）
  - 中转站文件 demo_deploy.sh
  - 演示剧本 docs-demo.yaml
  - 3 条分组执行任务（web/db/cache，充实任务历史）

用法：
  python3 scripts/serve-screenshots/seed_demo_data.py --password <admin密码> \
      [--base-url http://localhost:8080]
"""
import argparse
import json
import sys
import tempfile
import urllib.request

DEMO_USERS = [
    {"username": "bob", "password": "Demo12345!", "role": "editor"},
    {"username": "carol", "password": "Demo12345!", "role": "viewer"},
]
DEMO_PLAYBOOK = """# demo playbook for docs
name: Docs Demo
hosts: [web]
tasks:
  - name: echo
    action: shell echo hello
"""
DEMO_STAGING_SCRIPT = """#!/bin/bash
echo "hello from staging"
"""
EXEC_TASKS = [
    {"group": "web", "command": "uptime"},
    {"group": "db", "command": "df -h", "force": "true"},
    {"group": "cache", "script_content": "#!/bin/bash\necho \"disk check\"\ndf -h /",
     "script_name": "check_disk.sh", "force": "true"},
]


def call(base, path, token=None, data=None, raw=None, content_type=None, method=None):
    url = f"{base}/api/v1{path}"
    headers = {"Content-Type": content_type or "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    body = raw if raw is not None else (json.dumps(data).encode() if data else None)
    req = urllib.request.Request(url, data=body, headers=headers,
                                 method=method or ("POST" if body else "GET"))
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, json.loads(resp.read() or b"{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")


def multipart(path, name, content):
    boundary = "----owlshotsboundary"
    part = (f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="file"; filename="{name}"\r\n'
            f"Content-Type: application/octet-stream\r\n\r\n").encode() + content + \
        f"\r\n--{boundary}--\r\n".encode()
    return path, part, f"multipart/form-data; boundary={boundary}"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base-url", default="http://localhost:8080")
    ap.add_argument("--user", default="admin")
    ap.add_argument("--password", required=True)
    args = ap.parse_args()
    base = args.base_url

    status, resp = call(base, "/login", data={"username": args.user, "password": args.password})
    if status != 200:
        print(f"login failed: {status} {resp}")
        return 1
    token = resp["token"]
    print("[ok] login")

    status, resp = call(base, "/nodes/seed", token=token, method="POST")
    print(f"[ok] seed nodes: {resp}")

    for u in DEMO_USERS:
        status, resp = call(base, "/users", token=token, data=u)
        tag = "skip" if status == 409 else f"created id={resp.get('id')}"
        print(f"[ok] user {u['username']}: {status} {tag}")

    path, body, ctype = multipart("/staging/upload", "demo_deploy.sh", DEMO_STAGING_SCRIPT.encode())
    status, resp = call(base, path, token=token, raw=body, content_type=ctype)
    print(f"[ok] staging upload: {status} {resp.get('name', resp)}")

    path, body, ctype = multipart("/playbooks/upload", "docs-demo.yaml", DEMO_PLAYBOOK.encode())
    status, resp = call(base, path, token=token, raw=body, content_type=ctype)
    print(f"[ok] playbook upload: {status} {resp.get('data', {}).get('name', resp)}")

    for t in EXEC_TASKS:
        status, resp = call(base, "/exec", token=token, data=t)
        n = len(resp.get("tasks", []))
        print(f"[ok] exec {t.get('group')}: {n} task(s)")
    print("done: 等待约 60s 让任务进入 failed 终态后截图效果最佳")
    return 0


if __name__ == "__main__":
    sys.exit(main())
