# anvillm-inbox-notify runit service

Runit service for the [`anvillm-inbox-notify`](../../scripts/anvillm-inbox-notify) daemon.

## What it does

Streams the AnviLLM event bus (`9p read anvillm/events`) and fires desktop
notifications via `notify-send` when messages of configured types arrive in
the user's inbox.

Default notification types: `APPROVAL_REQUEST`, `REVIEW_REQUEST`.

Configurable via `~/.config/anvillm/notifications.yaml`:

```yaml
notify_on:
  - APPROVAL_REQUEST
  - REVIEW_REQUEST
  # - QUERY_REQUEST    # uncomment to also notify on queries
```

## Installation

### System-wide (runit)

```sh
sudo cp -r services/inbox-notify-runit /etc/sv/anvillm-inbox-notify
sudo ln -s /etc/sv/anvillm-inbox-notify /var/service/anvillm-inbox-notify
```

### Per-user (runit with ~/.config/service)

```sh
mkdir -p ~/.config/service
cp -r services/inbox-notify-runit ~/.config/service/anvillm-inbox-notify
sv up anvillm-inbox-notify
```

### Manual (foreground)

```sh
./scripts/anvillm-inbox-notify
```

## Prerequisites

- `anvillm-inbox-notify` script installed in `PATH`
- `notify-send` (from `libnotify` or `dunst`)
- `jq` ≥ 1.6
- `9p` command connected to a running `anvillm`
