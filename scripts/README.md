# Backend Wrapper Scripts

These scripts provide convenient launchers for acme-q with different backends.

## Usage

```sh
# Start with kiro-cli backend
./Kiro

# Start with claude backend
./Claude
```

## How They Work

Each script:
1. Locates the acme-q binary (checks `../acme-q`, `../Q`, `$home/bin/Q`, or PATH)
2. Executes it with the appropriate backend as a positional argument

## Direct Usage

You can also invoke acme-q directly with a backend name:

```sh
# Using kiro-cli (default)
acme-q
acme-q kiro-cli

# Using claude
acme-q claude
```

## Adding New Backends

To add a new backend:

1. Implement the backend in `internal/backends/`
2. Register it in `main.go`'s switch statement
3. Create a wrapper script following the pattern:

```rc
#!/usr/bin/env rc
# ... (location detection logic) ...
exec $acmeq your-backend-name
```
