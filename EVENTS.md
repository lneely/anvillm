# Events System

The events system provides a real-time text stream of all AnviLLM activity.

## Reading Events

```sh
9p read agent/events
```

This blocks and streams JSON events as they occur, one per line.

## Event Format

Each line is a JSON object:

```json
{"id":"uuid","ts":1708598520,"agent":"a1b2c3d4","type":"StateChange","data":{"state":"running"}}
{"id":"uuid","ts":1708598525,"agent":"a1b2c3d4","type":"UserRecv","data":{"from":"user","subject":"Review request"}}
```

## Event Types

- `StateChange` - Session state transitions (starting, idle, running, stopped, error, exited)
- `UserRecv` - Message received by user
- `UserSend` - Message sent by user
- `BotRecv` - Message received by bot
- `BotSend` - Message sent by bot

## Consuming Events

Pipe to any tool:

```sh
# Log to file
9p read agent/events >> anvillm.log

# Filter specific events
9p read agent/events | grep StateChange

# Parse with jq
9p read agent/events | jq 'select(.type == "UserRecv")'

# Custom notification script
9p read agent/events | while read event; do
  echo "$event" | jq -r '"\(.type): \(.agent)"'
done

# Desktop notifications on state changes
9p read agent/events | jq -r 'select(.type == "StateChange") | "\(.agent): \(.data.state)"' | \
  while read msg; do notify-send "AnviLLM" "$msg"; done
```

It's just a text stream — wire it however you want.

