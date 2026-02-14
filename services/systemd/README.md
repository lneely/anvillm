# systemd Service Units for anvilsrv

This directory contains systemd service unit files for running anvilsrv as a system service.

## Service Files

### 1. `anvilsrv-user.service` - User Service (Recommended)

Run anvilsrv as a user service using systemd's user instance.

**Installation:**
```sh
# Copy the service file
mkdir -p ~/.config/systemd/user
cp services/systemd/anvilsrv-user.service ~/.config/systemd/user/

# Reload systemd user daemon
systemctl --user daemon-reload

# Enable and start the service
systemctl --user enable anvilsrv-user.service
systemctl --user start anvilsrv-user.service

# Check status
systemctl --user status anvilsrv-user.service
```

**Management:**
- Start: `systemctl --user start anvilsrv-user`
- Stop: `systemctl --user stop anvilsrv-user`
- Restart: `systemctl --user restart anvilsrv-user`
- Status: `systemctl --user status anvilsrv-user`
- Logs: `journalctl --user -u anvilsrv-user -f`
- Disable: `systemctl --user disable anvilsrv-user`

**Enable Linger (Start on Boot):**

To make the service start on system boot even when not logged in:
```sh
sudo loginctl enable-linger $USER
```

### 2. `anvilsrv@.service` - System Service Template

Run anvilsrv as a system service for specific users.

**Installation:**
```sh
# Copy the service file
sudo cp services/systemd/anvilsrv@.service /etc/systemd/system/

# Reload systemd daemon
sudo systemctl daemon-reload

# Enable and start for a specific user (replace 'username')
sudo systemctl enable anvilsrv@username.service
sudo systemctl start anvilsrv@username.service

# Check status
sudo systemctl status anvilsrv@username.service
```

**Management:**
- Start: `sudo systemctl start anvilsrv@username`
- Stop: `sudo systemctl stop anvilsrv@username`
- Restart: `sudo systemctl restart anvilsrv@username`
- Status: `sudo systemctl status anvilsrv@username`
- Logs: `sudo journalctl -u anvilsrv@username -f`
- Disable: `sudo systemctl disable anvilsrv@username`

### 3. `anvilsrv.service` - Simple System Service

Basic system service (less flexible than the template).

**Installation:**
```sh
# Edit the service file to set the correct username
nano services/systemd/anvilsrv.service

# Copy the service file
sudo cp services/systemd/anvilsrv.service /etc/systemd/system/

# Reload systemd daemon
sudo systemctl daemon-reload

# Enable and start the service
sudo systemctl enable anvilsrv.service
sudo systemctl start anvilsrv.service

# Check status
sudo systemctl status anvilsrv.service
```

## Choosing the Right Service

- **User Service (`anvilsrv-user.service`)**: Best for most users. Runs in your user session, easier to manage, no sudo required.
- **System Service Template (`anvilsrv@.service`)**: Good for multi-user systems or when you need system-level control.
- **Simple System Service (`anvilsrv.service`)**: Basic system service, less flexible.

## Troubleshooting

### Check Logs
```sh
# User service
journalctl --user -u anvilsrv-user -n 50

# System service
sudo journalctl -u anvilsrv@username -n 50
```

### Verify Service is Running
```sh
# User service
systemctl --user status anvilsrv-user

# System service
sudo systemctl status anvilsrv@username
```

### Check if anvilsrv is Actually Running
```sh
anvilsrv status
```

### Restart After Code Changes
```sh
# User service
systemctl --user restart anvilsrv-user

# System service
sudo systemctl restart anvilsrv@username
```

## Notes

- The service automatically restarts on failure with a 5-second delay
- PID file is cleaned up on service stop
- Logs are available via journalctl
- The service uses the anvilsrv binary from `~/bin/anvilsrv`
