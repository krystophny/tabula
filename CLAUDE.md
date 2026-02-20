# CLAUDE

## Fast Path Rule

For direct runtime requests, run the obvious command first, then verify.
Do not scan source/docs unless the command fails.

## Start Local Web UI In Temporary Directory

Use this exact sequence:

```bash
TMP_ROOT="$(mktemp -d -t tabula-web-XXXXXX)"
PROJECT_DIR="$TMP_ROOT/project"
DATA_DIR="$TMP_ROOT/data"
LOG_FILE="$TMP_ROOT/web.log"
nohup go run ./cmd/tabula web \
  --project-dir "$PROJECT_DIR" \
  --data-dir "$DATA_DIR" \
  --host 127.0.0.1 \
  --port 8420 >"$LOG_FILE" 2>&1 &
PID=$!
curl -fsS http://127.0.0.1:8420/api/setup
```

Report back:
- URL: `http://127.0.0.1:8420`
- PID: `$PID`
- temp root/project/data/log paths

Stop command:

```bash
kill "$PID"
```

## Cross-Repo Protocol

The generic handoff protocol is maintained in:

- `../handoff-protocol`
- GitHub: `github.com/krystophny/handoff-protocol`

Use that repo as the source of truth for handoff envelope/schema/lifecycle (`handoff.create|peek|consume|revoke|status`).
