# AI Agent Operating Rules

## Admin Password Recovery
- NEVER delete `~/.owl/owl.db` to trigger first-run password output.
- Use `--reset-admin` flag instead:
  ```bash
  ./build/owl-serve --reset-admin --port 8080
  ```
  This resets the admin password, prints the new credentials, and starts the server in one command.

## Seed Data for Testing
- After server starts, seed 50 mock nodes by calling:
  ```bash
  TOKEN="$(curl -s http://localhost:8080/api/v1/login -X POST -H 'Content-Type: application/json' -d '{"username":"admin","password":"<password>"}' | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')"
  curl -X POST http://localhost:8080/api/v1/nodes/seed -H "Authorization: Bearer $TOKEN"
  ```
- This creates 50 nodes with diverse groups (web, db, cache, worker, monitor, gateway) and labels.
