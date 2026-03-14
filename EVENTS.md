# Events System

The events system provides a real-time text stream of all AnviLLM activity using [github.com/simonfxr/pubsub](https://github.com/simonfxr/pubsub) as the event bus.

## Architecture

<p align="center"><img src="diagrams/events.svg?v=2" width="400"></p>

Multiple clients can read from `anvillm/events` simultaneously, each consuming the same event stream for different purposes: notifications (desktop, SMS, WhatsApp), logging/auditing, custom integrations, etc.

## Reading Events

```sh
9p read anvillm/events
```

This blocks and streams JSON events as they occur, one per line.

## Event Format

Each line is a JSON object:

```json
{"id":"uuid","ts":1708598520,"source":"a1b2c3d4","type":"StateChange","data":{"state":"running"}}
{"id":"uuid","ts":1708598525,"source":"a1b2c3d4","type":"UserRecv","data":{"from":"user","subject":"Review request"}}
{"id":"uuid","ts":1708598530,"source":"beads/myproject","type":"BeadReady","data":{"id":"bd-abc","title":"...","status":"open","labels":[...],"comments":[...],"mount":"myproject",...}}
```

## Event Types

- `StateChange` - Session state transitions (starting, idle, running, stopped, error, exited)
- `UserRecv` - Message received by user
- `UserSend` - Message sent by user
- `BotRecv` - Message received by bot
- `BotSend` - Message sent by bot
- `BeadReady` - A bead transitioned to open/ready; `source` is `beads/<mount>`, `data` is full bead JSON including comments
- `BeadClaimed` - A bead was claimed by an agent; `source` is `beads/<mount>`, `data` is `{"bead_id","assignee","mount"}`

## Consuming Events

Pipe to any tool:

```sh
# Log to file
9p read anvillm/events >> anvillm.log

# Filter specific events
9p read anvillm/events | grep StateChange

# Parse with jq
9p read anvillm/events | jq 'select(.type == "UserRecv")'

# Custom notification script
9p read anvillm/events | while read event; do
  echo "$event" | jq -r '"\(.type): \(.source)"'
done

# Desktop notifications on state changes
9p read anvillm/events | jq -r 'select(.type == "StateChange") | "\(.source): \(.data.state)"' | \
  while read msg; do notify-send "AnviLLM" "$msg"; done
```

It's just a text stream — wire it however you want.

