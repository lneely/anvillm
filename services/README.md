# AnviLLM Service Management

This directory contains service management scripts for running `anvilsrv` as a system service.

## Available Init Systems

### systemd (Most Common)

For modern Linux distributions using systemd (Ubuntu, Fedora, Arch, Debian, etc.).

See [systemd/README.md](systemd/README.md) for installation instructions.

**Quick Start (User Service):**
```sh
mkdir -p ~/.config/systemd/user
cp services/systemd/anvilsrv-user.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now anvilsrv-user.service
```

### runit

For systems using runit (Void Linux, some custom setups).

See [runit/README.md](runit/README.md) for installation instructions.

**Quick Start:**
```sh
sudo cp -r services/runit /etc/sv/anvilsrv
sudo chmod +x /etc/sv/anvilsrv/run /etc/sv/anvilsrv/finish
sudo ln -s /etc/sv/anvilsrv /var/service/anvilsrv
```

## Manual Management (No Init System)

If you don't want to use an init system, you can manage anvilsrv manually:

```sh
# Start the server
anvilsrv start

# Check status
anvilsrv status

# Stop the server
anvilsrv stop
```

You can also add this to your shell startup file (`.profile`, `.bashrc`, etc.):
```sh
# Auto-start anvilsrv if not running
anvilsrv status 2>/dev/null || anvilsrv start
```

## Choosing an Init System

- **systemd**: Use if your distribution uses systemd (most modern Linux distributions)
- **runit**: Use if you're on Void Linux or specifically use runit
- **manual**: Use if you want full control or are testing/developing

## Troubleshooting

### Check if anvilsrv is Running
```sh
anvilsrv status
```

### Check 9P Socket
```sh
# Should show the socket file
ls -la /tmp/ns.$USER/agent

# Test connection
echo "list" | 9p read agent/list
```

### View Logs

**systemd:**
```sh
journalctl --user -u anvilsrv-user -f
```

**runit:**
```sh
sudo svlogd /var/log/anvilsrv
```

**manual:**
Check stderr output (anvilsrv logs to stderr/stdout by default).
