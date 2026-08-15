# AI Agent Operating Rules

## Admin Password Recovery
- NEVER delete `~/.owl/owl.db` to trigger first-run password output.
- Use `--reset-admin` flag instead:
  ```bash
  ./build/owl-serve --reset-admin --port 8080
  # 或通过 owl CLI 同样支持：
  owl serve --reset-admin --port 8080
  ```
  This resets the admin password, prints the new credentials, and starts the server in one command.

## Seed Data for Testing
- After server starts, seed 50 mock nodes by calling:
  ```bash
  TOKEN="$(curl -s http://localhost:8080/api/v1/login -X POST -H 'Content-Type: application/json' -d '{"username":"admin","password":"<password>"}' | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')"
  curl -X POST http://localhost:8080/api/v1/nodes/seed -H "Authorization: Bearer $TOKEN"
  ```
- This creates 50 nodes with diverse groups (web, db, cache, worker, monitor, gateway) and labels.

## No Plan No Implement
## test-driven-development
use /tdd 
## Git commit After E2E
after a success E2E , commit as atomic as possible .

✅ 允许操作：
- 读取、列出、修改当前项目根目录以及所有它下面的子文件夹、文件
- 使用 ./ 相对路径访问项目内部资源

❌ 严格禁止，一次都不能执行：
1. 使用 ../、../../ 等任何向上跳转父目录操作
2. cd ..，切换到上级目录
3. find、ls、grep 的搜索起点指向项目外目录
4. 访问系统目录：/etc、/home、/usr、/tmp、C:\Windows等
5. 使用绝对路径访问项目以外文件
6. 通过环境变量、命令拼接、符号链接等间接跳出项目目录
7. 试探性浏览上级文件夹，即使仅查看目录名称

当你需要某个文件但是在当前项目内找不到：不要去上层目录搜索，直接告知用户文件缺失，请用户提供正确路径。
如你生成的命令违反目录访问限制，执行器会直接拦截报错。