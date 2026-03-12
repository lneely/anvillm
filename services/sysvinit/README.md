# SysV init scripts

SysV init scripts for AnviLLM services, in two flavours:

| File | Style | Install at |
|------|-------|-----------|
| `anvilsrv` | Generic SysV + LSB INIT INFO header | `/etc/init.d/anvilsrv` |
| `rc.anvilsrv` | Slackware rc | `/etc/rc.d/rc.anvilsrv` |
| `anvillm-inbox-notify` | Generic SysV + LSB INIT INFO header | `/etc/init.d/anvillm-inbox-notify` |
| `rc.anvillm-inbox-notify` | Slackware rc | `/etc/rc.d/rc.anvillm-inbox-notify` |

---

## anvilsrv — AnviLLM backend server

`anvilsrv` is the core backend daemon.  It exposes the `anvillm/` 9P filesystem
used by Claude Code sessions, bots, the beads task store, and the event bus.

The binary (`~/bin/anvilsrv`) handles daemonization, PID file management, and
a `status` subcommand itself, so these init scripts delegate to `anvilsrv
start`, `anvilsrv stop`, and `anvilsrv status` rather than doing their own
process tracking.  The PID file lives at `$NAMESPACE/anvilsrv.pid`.

### Generic SysV (`/etc/init.d/anvilsrv`)

```sh
# Install
sudo install -m 755 services/sysvinit/anvilsrv /etc/init.d/

# Configure — set the user to run as
sudo sh -c 'echo "ANVILSRV_USER=yourlogin" > /etc/default/anvilsrv'
# Optionally override NAMESPACE (defaults to /tmp/ns.$ANVILSRV_USER):
# echo "NAMESPACE=/tmp/ns.yourlogin.:0" >> /etc/default/anvilsrv

# Register with the init system
sudo update-rc.d anvilsrv defaults   # Debian / Ubuntu
# -or-
sudo chkconfig --add anvilsrv        # Red Hat / CentOS

# Manage
sudo service anvilsrv start
sudo service anvilsrv stop
sudo service anvilsrv restart
sudo service anvilsrv status
```

### Slackware (`/etc/rc.d/rc.anvilsrv`)

```sh
# Install binary (built from source or downloaded)
install -m 755 /path/to/anvilsrv /home/yourlogin/bin/

# Install rc script
install -m 755 services/sysvinit/rc.anvilsrv /etc/rc.d/

# Set the user — edit ANVILSRV_USER= at the top of the script, or pass it:
ANVILSRV_USER=yourlogin /etc/rc.d/rc.anvilsrv start
```

Enable/disable (Slackware convention):

```sh
chmod 755 /etc/rc.d/rc.anvilsrv   # enable
chmod 644 /etc/rc.d/rc.anvilsrv   # disable
```

Add to `/etc/rc.d/rc.local` to start at boot:

```sh
if [ -x /etc/rc.d/rc.anvilsrv ]; then
  /etc/rc.d/rc.anvilsrv start
fi
```

Add to `/etc/rc.d/rc.local_shutdown` to stop cleanly:

```sh
if [ -x /etc/rc.d/rc.anvilsrv ]; then
  /etc/rc.d/rc.anvilsrv stop
fi
```

---

## anvillm-inbox-notify — inbox notification daemon

`anvillm-inbox-notify` streams the AnviLLM event bus and fires `notify-send`
desktop notifications when messages of configured types (default:
`APPROVAL_REQUEST`, `REVIEW_REQUEST`) arrive in the user's inbox.  Configure
in `~/.config/anvillm/notifications.yaml`.

Because this daemon needs a graphical session (`DISPLAY`, `DBUS_SESSION_BUS_ADDRESS`),
both scripts auto-detect those values from `/proc/<pid>/environ` for
`ANVILLM_USER`.  See the note at the end of this file.

### Generic SysV (`/etc/init.d/anvillm-inbox-notify`)

```sh
# Install daemon and init script
sudo install -m 755 scripts/anvillm-inbox-notify /usr/local/bin/
sudo install -m 755 services/sysvinit/anvillm-inbox-notify /etc/init.d/

# Configure
sudo sh -c 'echo "ANVILLM_USER=yourlogin" > /etc/default/anvillm-inbox-notify'

# Register
sudo update-rc.d anvillm-inbox-notify defaults
# -or-
sudo chkconfig --add anvillm-inbox-notify

# Manage
sudo service anvillm-inbox-notify start
sudo service anvillm-inbox-notify stop
sudo service anvillm-inbox-notify restart
sudo service anvillm-inbox-notify status
```

### Slackware (`/etc/rc.d/rc.anvillm-inbox-notify`)

```sh
# Install daemon
install -m 755 scripts/anvillm-inbox-notify /usr/local/bin/

# Install rc script
install -m 755 services/sysvinit/rc.anvillm-inbox-notify /etc/rc.d/

# Set user: edit ANVILLM_USER= at the top, or pass it:
ANVILLM_USER=yourlogin /etc/rc.d/rc.anvillm-inbox-notify start
```

Enable/disable and rc.local hooks follow the same pattern as rc.anvilsrv above.

---

## Note on DISPLAY / DBUS (inbox-notify only)

`anvillm-inbox-notify` fires desktop notifications and therefore needs the
target user's X11 `DISPLAY` and `DBUS_SESSION_BUS_ADDRESS`.  Both scripts
discover these automatically by scanning `/proc/<pid>/environ` for processes
owned by `ANVILLM_USER`, falling back to `DISPLAY=:0` and the systemd user
bus path (`/run/user/<uid>/bus`) if nothing is found.

If the X session hasn't started when the script runs the daemon will use the
fallback values; `notify-send` will connect successfully for a standard single-
seat setup where X is already running.

For a fully session-integrated approach (no guesswork), prefer the
[systemd user service](../systemd/anvillm-inbox-notify.service) or the
[runit service](../inbox-notify-runit/).
