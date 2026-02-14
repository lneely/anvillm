---
name: gh-cli
description: Interact with GitHub repositories, issues, PRs, workflows, and more using the gh CLI. Use for GitHub operations like creating issues/PRs, managing repos, checking workflow runs, and browsing GitHub resources.
---

# GitHub CLI (gh)

## Purpose

Execute GitHub operations directly from the command line using the `gh` CLI tool.

## When to Use

- Creating, viewing, or managing issues and pull requests
- Repository operations (clone, create, view, fork)
- Checking GitHub Actions workflow runs and caches
- Managing releases, gists, and secrets
- Searching GitHub for repos, issues, or PRs
- Opening GitHub resources in the browser
- Making authenticated GitHub API calls

## Core Commands

### Repository Management
```bash
gh repo create <name>              # Create a new repository
gh repo clone <repo>               # Clone a repository
gh repo view [repo]                # View repository details
gh repo fork [repo]                # Fork a repository
```

### Issues
```bash
gh issue create                    # Create a new issue
gh issue list                      # List issues
gh issue view <number>             # View issue details
gh issue close <number>            # Close an issue
```

### Pull Requests
```bash
gh pr create                       # Create a new PR
gh pr list                         # List PRs
gh pr view <number>                # View PR details
gh pr checkout <number>            # Checkout a PR locally
gh pr merge <number>               # Merge a PR
gh pr status                       # Show status of PRs
```

### GitHub Actions
```bash
gh run list                        # List workflow runs
gh run view <run-id>               # View run details
gh run watch <run-id>              # Watch a run in real-time
gh workflow list                   # List workflows
gh workflow view <workflow>        # View workflow details
```

### Releases
```bash
gh release create <tag>            # Create a release
gh release list                    # List releases
gh release view <tag>              # View release details
```

### Search
```bash
gh search repos <query>            # Search repositories
gh search issues <query>           # Search issues
gh search prs <query>              # Search pull requests
```

### Browse
```bash
gh browse                          # Open repo in browser
gh browse <number>                 # Open issue/PR in browser
```

### API Access
```bash
gh api <endpoint>                  # Make authenticated API request
gh api repos/:owner/:repo/issues   # Example: list issues via API
```

## Common Flags

- `--repo <owner/repo>` - Specify repository
- `--json` - Output as JSON
- `--jq <query>` - Filter JSON output with jq
- `--web` - Open in web browser
- `--help` - Show help for any command

## Authentication

Check authentication status:
```bash
gh auth status
```

Login (if needed):
```bash
gh auth login
```

## Examples

```bash
# Create an issue in current repo
gh issue create --title "Bug: Login fails" --body "Description here"

# List open PRs with JSON output
gh pr list --state open --json number,title,author

# View workflow run details
gh run view 12345

# Clone a repository
gh repo clone owner/repo

# Search for Rust repositories
gh search repos --language rust --sort stars

# Make a custom API call
gh api graphql -f query='query { viewer { login } }'
```

## Notes

- Most commands work in the context of the current git repository
- Use `--repo owner/name` to operate on a different repository
- Authentication is required for most operations
- Use `gh <command> --help` for detailed command documentation
