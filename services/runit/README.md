# runit Service for anvilsrv

This directory contains a runit service definition for anvilsrv.

## Installation

1. Copy this directory to `/etc/sv/anvilsrv`:
   ```sh
   sudo cp -r services/runit /etc/sv/anvilsrv
   sudo chmod +x /etc/sv/anvilsrv/run
   sudo chmod +x /etc/sv/anvilsrv/finish
   ```

2. Enable the service by creating a symlink:
   ```sh
   sudo ln -s /etc/sv/anvilsrv /var/service/anvilsrv
   ```

3. The service will start automatically. Check status:
   ```sh
   sudo sv status anvilsrv
   ```

## Service Management

- **Start**: `sudo sv start anvilsrv`
- **Stop**: `sudo sv stop anvilsrv`
- **Restart**: `sudo sv restart anvilsrv`
- **Status**: `sudo sv status anvilsrv`
- **View logs**: `sudo svlogd /var/log/anvilsrv`

## Notes

- The service runs as the current user specified in the run script
- Logs are written to stderr and can be captured using svlogd
- The service will automatically restart if it crashes
- To disable the service, remove the symlink from /var/service:
  ```sh
  sudo rm /var/service/anvilsrv
  ```
