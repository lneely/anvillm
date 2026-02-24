# Capability Level Classification

Capability levels control which model tier a bot session uses when working a bead.
They encode task difficulty, not task clarity. Clarity is handled separately by the
enrichment gate (bd-3bu.5): a bead that fails the cold-completion test must be enriched
before any capability level assignment matters.

Guiding principle: **use the minimum capable model.** Cost difference is ~10-20× per tier.
Never assign `high` to anything a `low` model can handle. When in doubt, assign lower.

---

## 1. Technical Difficulty Axis

Rate the inherent difficulty of the work, independent of how well the bead is written.

### Low difficulty

Mechanical operations with a clear, bounded change surface. No novel reasoning required.
The bot needs only to follow an obvious existing pattern or perform a single operation.

Examples:
- Create a bead, update a field, send a message, add a label
- Rename a function or variable (project-wide or single-file)
- Add a struct field and wire it through one obvious call site
- Update a config value or constant
- Fix a typo or trivial off-by-one
- Write a short new function that clearly mirrors an existing one
- Add a case to an existing switch/dispatch table

### Standard difficulty

Moderate reasoning required. Multi-step change with some judgment calls, but within
well-understood patterns. The bot will explore 2–5 files and make coordinated edits.

Examples:
- Thread a new parameter through multiple layers (as in bd-frk.12)
- Implement a new 9P file endpoint following the existing pattern
- Add a feature to an existing subsystem (e.g., extend a parser, add a CLI flag)
- Write unit tests for an existing package
- Refactor a module while keeping the public API stable
- Debug a non-trivial bug where the root cause is unclear

### High difficulty

Requires significant novel reasoning, design, or understanding of complex system dynamics.
The bot must reason about concurrency, state invariants, performance trade-offs, or
architectural fit across many components.

Examples:
- Implement a new concurrent primitive or lock-free data structure
- Design and implement a caching layer with invalidation semantics
- Write a novel algorithm (e.g., dependency resolution, topological sort with cycles)
- Implement a complex state machine with many transitions and invariants
- Significant architectural change touching many subsystems
- Debugging a race condition or subtle memory safety issue
- Designing a protocol or API where the trade-offs are non-obvious

---

## 2. Capability Level Taxonomy

Combine the clarity gate with the difficulty rating to produce a capability level.

| Clarity gate | Difficulty | Capability level | Action |
|---|---|---|---|
| Fails | Any | — | Enrich first (bd-3bu.5 flow), then re-rate |
| Passes | Low | `capability:low` | Assign to haiku |
| Passes | Standard | `capability:standard` | Assign to sonnet |
| Passes | High | `capability:high` | Assign to opus |

**Default:** when no label is present, treat as `capability:standard`.

**When in doubt:** assign the lower level. Sonnet handles almost all standard dev tasks.
Reserve `capability:high` for cases where the technical difficulty is unambiguous (e.g.,
the task description explicitly involves concurrency, novel algorithms, or cross-subsystem
architectural change).

Do not assign `capability:high` based on priority alone. A P1 bead may still be
`capability:low` if the work is structurally simple (e.g., "update the default timeout
constant in config.go").

---

## 3. Model Mapping per Backend

| Capability level | claude | kiro-cli | ollama |
|---|---|---|---|
| `capability:low` | haiku | haiku | smallest available model |
| `capability:standard` | sonnet | sonnet | mid-tier model |
| `capability:high` | opus | opus | largest available model |

For ollama: run `ollama list` to enumerate available models. Map smallest→low,
largest→high, middle→standard. If only one model is available, use it for all levels.

### Label convention

Labels use the portable level name, not the backend-specific model name:

```sh
echo "label bd-abc capability:low"      | 9p write agent/beads/ctl
echo "label bd-abc capability:standard" | 9p write agent/beads/ctl
echo "label bd-abc capability:high"     | 9p write agent/beads/ctl
```

The Conductor and Taskmaster read these labels and resolve the backend-specific model
name at spawn time using the table above. See bot-templates/Conductor (Model Tier Mapping
section) and bot-templates/Taskmaster for the label-to-model resolution logic.

---

## 4. Quick Reference

```
Low  → haiku:  task mgmt, field add, rename, config update, single-file mechanical edit
Std  → sonnet: multi-file feature, parameter threading, moderate debugging, new endpoint
High → opus:   concurrency, caching, novel algorithms, complex state machines, arch design
```

**Red flags for over-provisioning (do not assign high):**
- The task is "write a description" or "update documentation"
- The task is "add a label" or "create a bead"
- The task follows an obvious existing pattern (even if the pattern is multi-file)
- The bead priority is high but the work is structurally simple

---

## References

- bd-3bu.5: cold-completion gate (enrichment prerequisite)
- bd-9y4: vague-language lint
- bd-frk.7: capability label convention on beads
- bd-frk.10: model tier enumeration per backend
- bd-frk.11: Taskmaster applies capability labels at bead creation
- bd-frk.13: Conductor propagates model to worker sessions
