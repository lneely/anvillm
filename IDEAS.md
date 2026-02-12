# IDEAS.md

## Shared Context

Inject context from one session into another.

Possible approaches:
- Export conversation summary to a file, import into another session
- Shared 9p file that multiple sessions can read from
- Command to "copy context" between sessions via the orchestrator
- Named context snippets that can be referenced by any session

## Bot-to-Bot Communication

Two running bots talk directly via tmux send-keys.

The bots already automate shell commands - they could inject prompts into each other's sessions. Use cases:
- Specialist handoff: "ask the database bot to check the schema"
- Parallel exploration: one bot researches while another codes
- Adversarial review: bot A writes code, bot B critiques it

Implementation: expose other sessions' tmux targets via 9p or a /peer command. Bot reads peer's output, sends prompts, reads response.

## Pull Instead of Push

Current model: bot writes output to `in` fd for a given session.

Alternative: bot writes to `out` fd instead. Another bot (or UI) pulls from `out`. Status transitions "running" → "await" when bot writes to `out`.

### 9p FIFO Semantics

`out` is a FIFO implemented in 9p: buffers in memory until read, non-blocking writes. Next write clears and overwrites with new value. Unlike Unix FIFOs, we control the semantics - no writer blocking, no reader blocking on empty.

### Clearing `out`

Next write clears previous content. Window for lost output is small - only if consumer never reads before producer runs again. That's an orchestration bug, not a mechanism problem. Orchestrator assumes responsibility for coordination.

### Pull vs Push

- **Pull**: read from another bot's `out`. Consumer initiates.
- **Push**: write to another bot's `in`. Producer initiates.

Both are file ops over 9p. Status file signals readiness (`await` = output available).

### Fan-in / Fan-out

Works naturally:

**Fan-in**: bot reads from multiple `out` files. Example: dev bot reads reviewer1/out, reviewer2/out, ... reviewer5/out.

**Fan-out via push**: bot writes to multiple `in` files.

**Fan-out via pull**: bot writes to its `out`, multiple consumers read same file before next write clears it.

Example cycle (adversarial review):
1. 5 reviewers write to their `out`, enter `await`
2. Dev bot reads all 5 `out`s (fan-in), applies changes, writes diff to its `out`, enters `await`
3. Orchestrator tells all 5 reviewers: "read from dev/out"
4. All 5 read same `out` before dev runs again (fan-out via pull)
5. Repeat

Orchestrator sequences correctly - doesn't send new work to dev until all reviewers have pulled.
