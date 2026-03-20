# runit Service for anvillm

This directory contains a runit service definition for anvillm.

## Installation

1. Copy this directory to `/etc/sv/anvillm`:
   ```sh
   sudo cp -r services/runit /etc/sv/anvillm
   sudo chmod +x /etc/sv/anvillm/run
   sudo chmod +x /etc/sv/anvillm/finish
   ```

2. Enable the service by creating a symlink:
   ```sh
   sudo ln -s /etc/sv/anvillm /var/service/anvillm
   ```

3. The service will start automatically. Check status:
   ```sh
   sudo sv status anvillm
   ```

## Service Management

- **Start**: `sudo sv start anvillm`
- **Stop**: `sudo sv stop anvillm`
- **Restart**: `sudo sv restart anvillm`
- **Status**: `sudo sv status anvillm`
- **View logs**: `sudo svlogd /var/log/anvillm`

## Notes

- The service runs as the current user specified in the run script
- Logs are written to stderr and can be captured using svlogd
- The service will automatically restart if it crashes
- To disable the service, remove the symlink from /var/service:
  ```sh
  sudo rm /var/service/anvillm
  ```
