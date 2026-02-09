# Backend Wrapper Scripts

These scripts provide convenient launchers for acme-q with different backends.

## Usage

```sh
# Start with kiro-cli backend
./Kiro

# Start with claude backend
./Claude
```

## Direct Usage

You can also invoke Assist directly with a backend name:

```sh
# Using kiro-cli (default)
Assist
Assist kiro-cli

# Using claude
Assist claude
```

## Adding New Backends

To add a new backend:

1. Implement the backend in `internal/backends/`
2. Register it in `main.go`'s switch statement
3. Create a wrapper script following the pattern:

```rc
#!/usr/bin/env rc
Assist your-backend-name
```
