# Toys

Small utilities in `scripts/` that build on the 9P API.

## inbox_refresh.py

Auto-refreshes the `/AnviLLM/inbox` window in Acme whenever a new message arrives. Listens to `anvillm/events` for `UserRecv` events and rewrites the inbox window body via the Acme 9P filesystem.

```sh
python3 scripts/inbox_refresh.py
```

Requires an open `/AnviLLM/inbox` window in Acme.

## metrics-dashboard.sh

Dashboard for `anvilmcp` execution metrics. Reads JSONL logs from `~/.local/state/anvilmcp/` and reports execution statistics, security events, and validation failures.

```sh
bash scripts/metrics-dashboard.sh
```

Reports:
- Execution counts, success/failure rates, average duration
- Executions by language
- Security events (grouped by type, with recent details)
- Recent errors and validation failures

## msgtrace.py

Real-time message sequence diagram generator. Listens to `anvillm/events` for `UserSend`/`BotSend` events and renders a PlantUML sequence diagram in a local web UI. Useful for visualizing inter-agent communication.

```sh
python3 scripts/msgtrace.py
# Open http://localhost:8089
```

The web UI auto-refreshes every 2 seconds, supports pan/zoom, and has a reset button to clear the trace. Requires `plantuml` in PATH.
