# systemd Service Units for anvillm

This directory contains systemd service unit files for running anvillm as a system service.

## Service Files

### 1. `anvillm-user.service` - User Service (Recommended)

Run anvillm as a user service using systemd's user instance.

**Installation:**
```sh
# Copy the service file
mkdir -p ~/.config/systemd/user
cp services/systemd/anvillm-user.service ~/.config/systemd/user/

# Reload systemd user daemon
systemctl --user daemon-reload

# Enable and start the service
systemctl --user enable anvillm-user.service
systemctl --user start anvillm-user.service

# Check status
systemctl --user status anvillm-user.service
```

**Management:**
- Start: `systemctl --user start anvillm-user`
- Stop: `systemctl --user stop anvillm-user`
- Restart: `systemctl --user restart anvillm-user`
- Status: `systemctl --user status anvillm-user`
- Logs: `journalctl --user -u anvillm-user -f`
- Disable: `systemctl --user disable anvillm-user`

**Enable Linger (Start on Boot):**

To make the service start on system boot even when not logged in:
```sh
sudo loginctl enable-linger $USER
```

### 2. `anvillm@.service` - System Service Template

Run anvillm as a system service for specific users.

**Installation:**
```sh
# Copy the service file
sudo cp services/systemd/anvillm@.service /etc/systemd/system/

# Reload systemd daemon
sudo systemctl daemon-reload

# Enable and start for a specific user (replace 'username')
sudo systemctl enable anvillm@username.service
sudo systemctl start anvillm@username.service

# Check status
sudo systemctl status anvillm@username.service
```

**Management:**
- Start: `sudo systemctl start anvillm@username`
- Stop: `sudo systemctl stop anvillm@username`
- Restart: `sudo systemctl restart anvillm@username`
- Status: `sudo systemctl status anvillm@username`
- Logs: `sudo journalctl -u anvillm@username -f`
- Disable: `sudo systemctl disable anvillm@username`

### 3. `anvillm.service` - Simple System Service

Basic system service (less flexible than the template).

**Installation:**
```sh
# Edit the service file to set the correct username
nano services/systemd/anvillm.service

# Copy the service file
sudo cp services/systemd/anvillm.service /etc/systemd/system/

# Reload systemd daemon
sudo systemctl daemon-reload

# Enable and start the service
sudo systemctl enable anvillm.service
sudo systemctl start anvillm.service

# Check status
sudo systemctl status anvillm.service
```

## Choosing the Right Service

- **User Service (`anvillm-user.service`)**: Best for most users. Runs in your user session, easier to manage, no sudo required.
- **System Service Template (`anvillm@.service`)**: Good for multi-user systems or when you need system-level control.
- **Simple System Service (`anvillm.service`)**: Basic system service, less flexible.

## Troubleshooting

### Check Logs
```sh
# User service
journalctl --user -u anvillm-user -n 50

# System service
sudo journalctl -u anvillm@username -n 50
```

### Verify Service is Running
```sh
# User service
systemctl --user status anvillm-user

# System service
sudo systemctl status anvillm@username
```

### Check if anvillm is Actually Running
```sh
anvillm status
```

### Restart After Code Changes
```sh
# User service
systemctl --user restart anvillm-user

# System service
sudo systemctl restart anvillm@username
```

## Notes

- The service automatically restarts on failure with a 5-second delay
- PID file is cleaned up on service stop
- Logs are available via journalctl
- The service uses the anvillm binary from `~/bin/anvillm`
