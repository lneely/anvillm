# AnviLLM Service Management

This directory contains service management scripts for running `anvillm` as a system service.

## Available Init Systems

### systemd (Most Common)

For modern Linux distributions using systemd (Ubuntu, Fedora, Arch, Debian, etc.).

See [systemd/README.md](systemd/README.md) for installation instructions.

**Quick Start (User Service):**
```sh
mkdir -p ~/.config/systemd/user
cp services/systemd/anvillm-user.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now anvillm-user.service
```

### runit

For systems using runit (Void Linux, some custom setups).

See [runit/README.md](runit/README.md) for installation instructions.

**Quick Start:**
```sh
sudo cp -r services/runit /etc/sv/anvillm
sudo chmod +x /etc/sv/anvillm/run /etc/sv/anvillm/finish
sudo ln -s /etc/sv/anvillm /var/service/anvillm
```

## Manual Management (No Init System)

If you don't want to use an init system, you can manage anvillm manually:

```sh
# Start the server
anvillm start

# Check status
anvillm status

# Stop the server
anvillm stop
```

You can also add this to your shell startup file (`.profile`, `.bashrc`, etc.):
```sh
# Auto-start anvillm if not running
anvillm status 2>/dev/null || anvillm start
```

## Choosing an Init System

- **systemd**: Use if your distribution uses systemd (most modern Linux distributions)
- **runit**: Use if you're on Void Linux or specifically use runit
- **manual**: Use if you want full control or are testing/developing

## Troubleshooting

### Check if anvillm is Running
```sh
anvillm status
```

### Check 9P Socket
```sh
# Should show the socket file
ls -la /tmp/ns.$USER/agent

# Test connection
echo "list" | 9p read anvillm/list
```

### View Logs

**systemd:**
```sh
journalctl --user -u anvillm-user -f
```

**runit:**
```sh
sudo svlogd /var/log/anvillm
```

**manual:**
Check stderr output (anvillm logs to stderr/stdout by default).
