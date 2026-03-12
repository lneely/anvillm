# Workflow Design & Implementation Plan

## Summary

This document describes the current state and planned implementation of the
role-based workflow system in anvillm. It covers bot roles, team composition,
emergent delegation, and the Supervisor redesign.

---

## What Exists Today

### `anvilspawn` (`scripts/anvilspawn`)

Single primitive for spawning a bot with a role:

```
anvilspawn <backend> <role> <workdir>
```

- Computes an 8-character hex hash of `$WORKDIR` using md5sum
- Spawns a new session: `echo "new $BACKEND $WORKDIR" | 9p write anvillm/ctl`
- Sets alias to `$HASH-$ROLE` (e.g. `a3f2b1c4-developer`)
- Assigns role via `echo "$ROLE" | 9p write anvillm/$ID/role`

Bots spawned for the same `$WORKDIR` share the same hash prefix, making them
discoverable to each other via `list_sessions`. This is what enables organic
collaboration without any explicit wiring.

### Roles (`roles/`)

All roles follow a standard structure:

```
## Responsibilities
## Prohibited Activities
## Workflow
## Response Format
```

Current roles:

| File | Description |
|------|-------------|
| `solo-developer.md` | Full-cycle: code, research, test, deploy, task mgmt |
| `developer.md` | Code only; no research, testing, or review |
| `reviewer.md` | Code review; automated checks + manual review |
| `tester.md` | Testing; unit, integration, fault injection, static analysis |
| `devops.md` | CI/CD, deployment, infrastructure |
| `researcher.md` | Query answering via three-tier cache (KB → code → web) |
| `taskmgr.md` | GitHub issues and PRs via `gh` CLI |
| `author.md` | Documentation writing |
| `technical-editor.md` | Editorial review of documentation |

**Key design decisions:**
- No explicit delegation protocols in any role — delegation is emergent
- No inbox-check startup steps — messages are delivered automatically
- `Prohibited Activities` sections drive emergent delegation: when a bot
  can't do something, it naturally looks for a peer that can

### Team Composition

There are no fixed team templates. Teams are assembled by running `anvilspawn`
once per role, using the same `$WORKDIR`. The hash ties them together.

Example — a dev+review+test team:
```sh
anvilspawn claude developer ~/src/myproject
anvilspawn claude reviewer ~/src/myproject
anvilspawn claude tester ~/src/myproject
```

All three share the same hash prefix and can find each other via `list_sessions`.

---

## Emergent Delegation

When a bot receives a task outside its `Prohibited Activities`, it naturally:
1. Checks `list_sessions` for a peer with a compatible role
2. Sends the appropriate message type (REVIEW_REQUEST, QUERY_REQUEST, etc.)
3. Waits for a response before proceeding

If no peer is available, the bot refuses the out-of-scope task rather than
attempting it. This is by design — the human decides team composition.

**Observed behavior:** a `devops` bot, upon receiving a coding task, derived
on its own that it should hand off to the `developer` bot. No explicit
delegation protocol was required.

---

## Bead Workflow

### Current State

Beads support:
- States: `open → in_progress → closed`
- Parent-child relationships (`new title description <parent-id>`)
- Dependencies (`dep`/`undep`, `beads/ready` queue)
- Capability labels: `capability:low|standard|high` (model tier hints)
- Arbitrary labels via `store.AddLabel`

Bots have write access to beads and can create child beads organically.

### Planned: `claimable-by` Label

A new label type: `claimable-by:<role>` (e.g. `claimable-by:reviewer`).

This label does two things:
1. Opts the bead into Supervisor dispatch (unlabeled beads are ignored)
2. Specifies which role must handle it

Beads without `claimable-by` are left for organic bot claiming or human
assignment. Both models coexist without conflict — the Supervisor only
touches what it has been told to dispatch.

**Usage:**
```sh
# When creating a bead that should be dispatched to a reviewer:
echo "new 'Review: auth module' 'description' bd-123 claimable-by:reviewer" \
  | 9p write anvillm/beads/ctl
```

The `capability` label is now optional and orthogonal to `claimable-by`.

---

## Supervisor Redesign

### Current State

The existing Supervisor (`bot-templates/Supervisor`, now deleted):
- Polls `beads/ready` every N seconds
- Dispatches by `capability_level` label → model tier
- Spawns new sessions (haiku/sonnet/opus) as needed
- Writes a generic "complete bead X" context to spawned sessions
- Reuses stopped sessions when model tier matches
- Cleans up stopped/exited sessions with closed beads

### Planned Changes

**Remove entirely:**
- Session spawning — `anvilspawn` is the human's tool for team composition
- Capability-level dispatch — optional, ignore for now
- Generic worker context writing — bots already have roles from `anvilspawn`

**Add:**
- `claimable-by:<role>` label reading from bead JSON
- Session matching by hash prefix AND role:
  1. Compute `HASH=$(echo -n "$WORKDIR" | md5sum | cut -c1-8)`
  2. For each session: check alias starts with `$HASH-`
  3. AND: `9p read anvillm/<id>/role` == required role
- Inbox delivery after claiming: write a PROMPT_REQUEST to the matched
  session's inbox notifying it of the bead assignment
- Skip (don't dispatch) if no matching session exists — leave bead queued

**Keep:**
- Polling loop with configurable interval
- Cleanup: kill stopped/exited sessions whose assigned beads are closed
- Skip beads that are already assigned

### New Dispatch Logic (pseudocode)

```
HASH = md5(WORKDIR)[0:8]

for each bead in beads/ready:
    if bead.assignee is not empty: skip
    role = extract_label(bead.labels, "claimable-by:")
    if role is empty: skip  # not supervisor-managed

    session = find_session(hash=HASH, role=role, state=running|idle)
    if session is empty: skip  # no bot available, leave queued

    claim(bead.id, session.id)
    deliver_to_inbox(session.id, bead.id)
```

### Session Matching

```bash
find_session() {
    local want_role="$1"
    while read -r sess_id backend state alias cwd; do
        [[ "$alias" == "$HASH-"* ]] || continue
        sess_role=$(9p read "anvillm/$sess_id/role" 2>/dev/null)
        [ "$sess_role" = "$want_role" ] || continue
        echo "$sess_id"
        return
    done < <(9p read anvillm/list)
}
```

### Inbox Delivery

After claiming, notify the bot via the user/outbox mechanism:
```
echo "to=$SESSION_ID type=PROMPT_REQUEST subject=bead:$BEAD_ID body=..." \
  | 9p write user/outbox
```

*(Exact format to be confirmed against the p9 server user/outbox write handler.)*

---

## Implementation Plan

### Phase 1 — `claimable-by` label support in beads (infrastructure)

- [ ] Add `claimable-by` as a recognized label prefix in `internal/p9/beads.go`
- [ ] Expose it in bead JSON output so the Supervisor can read it
- [ ] Verify `beads/ready` includes beads with `claimable-by` labels

### Phase 2 — Supervisor rewrite (`scripts/anvilsupervisor` or updated script)

- [ ] Remove spawning logic entirely
- [ ] Remove capability-level dispatch
- [ ] Implement `find_session` with hash+role matching
- [ ] Implement `claimable-by` label extraction from bead JSON
- [ ] Implement inbox delivery after claim
- [ ] Keep cleanup loop
- [ ] Update mkfile to install the new Supervisor

### Phase 3 — Role-aware bead creation

- [ ] Update `beads` skill to document `claimable-by` label
- [ ] Ensure bots that create child beads (e.g. developer creating a review
      bead) know to add `claimable-by:reviewer` so the Supervisor picks it up

### Phase 4 — Dependency gate enforcement (future)

- [ ] Supervisor should not dispatch a bead if its dependencies are not all
      `closed` (this may already be handled by `beads/ready` — verify)
- [ ] Consider adding a rule: parent bead cannot transition to `closed` unless
      all child beads with `claimable-by` labels are `closed`

---

## Open Questions

1. **user/outbox format** — confirm exact write format for delivering a message
   to a session inbox from outside an agent context (i.e. from the Supervisor
   shell script).

2. **beads/ready and dependencies** — does `beads/ready` already exclude beads
   with open dependencies, or does the Supervisor need to check this itself?

3. **Supervisor as a bot vs. script** — the current Supervisor is a polling
   shell script. This works but has no inbox. A future consideration is whether
   the Supervisor should itself be a session (bot) so it can receive
   notifications rather than polling. Not urgent.

4. **Multiple sessions with the same role** — if two `developer` bots share
   the same hash (e.g. two instances for the same workdir), which one gets the
   bead? Current plan: first match wins. Could be round-robin later.
