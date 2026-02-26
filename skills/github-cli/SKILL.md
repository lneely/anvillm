---
name: github-cli
intent: github, vcs
description: Interact with GitHub repositories, issues, PRs, workflows, and more using the gh CLI. Use for GitHub operations like creating issues/PRs, managing repos, checking workflow runs, and browsing GitHub resources.
---

# GitHub CLI (gh)

**CRITICAL:** Run `gh` commands from within the git repository directory. Use `--repo owner/name` only when operating on a different repo.

## Commands

**Repository:**
- Create repo: `gh repo create <name>`
- Clone repo: `gh repo clone <repo>`
- View repo: `gh repo view [repo]`
- Fork repo: `gh repo fork [repo]`

**Issues:**
- Create: `gh issue create --title "..." --body "..."`
- List: `gh issue list`
- View: `gh issue view <number>`
- Close: `gh issue close <number>`

**Pull Requests:**
- Create: `gh pr create`
- List: `gh pr list`
- View: `gh pr view <number>`
- Checkout: `gh pr checkout <number>`
- Merge: `gh pr merge <number>`
- Status: `gh pr status`

**Actions:**
- List runs: `gh run list`
- View run: `gh run view <run-id>`
- Watch run: `gh run watch <run-id>`
- List workflows: `gh workflow list`
- View workflow: `gh workflow view <workflow>`

**Releases:**
- Create: `gh release create <tag>`
- List: `gh release list`
- View: `gh release view <tag>`

**Search:**
- Repos: `gh search repos <query>`
- Issues: `gh search issues <query>`
- PRs: `gh search prs <query>`

**Browse:**
- Open repo: `gh browse`
- Open issue/PR: `gh browse <number>`

**API:**
- Call endpoint: `gh api <endpoint>`
- Example: `gh api repos/:owner/:repo/issues`

## Common Flags

`--repo owner/repo` `--json` `--jq <query>` `--web` `--help`

## Auth

- Check: `gh auth status`
- Login: `gh auth login`

## Examples

```bash
gh issue create --title "Bug" --body "Description"
gh pr list --state open --json number,title,author
gh run view 12345
gh repo clone owner/repo
gh search repos --language rust --sort stars
gh api graphql -f query='query { viewer { login } }'
```
