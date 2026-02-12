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

## Conductor Bot

A meta-bot that orchestrates workflows by watching status and dispatching prompts.

### Role

- Monitors status files of worker bots
- Issues coordination prompts when bots enter `await`
- Defines workflow through prompt injection, not hardcoded logic

### Example: Adversarial Review Cycle

Conductor's initial prompt:
```
You are coordinating a code review cycle between 5 reviewer bots and 1 dev bot.

Workflow:
1. Tell reviewers: "Review the code in dev/out, write feedback to your out"
2. Wait for all 5 reviewers to enter await
3. Tell dev: "Read reviewer1/out through reviewer5/out, apply feedback, write updated diff to out"
4. Wait for dev to enter await
5. Repeat from step 1 until reviewers report no issues

Monitor status files. Only proceed when all expected bots are in await.
```

Conductor reads status, sends prompts to `in` files, watches for `await`. The workflow is entirely prompt-driven.

### Benefits

- Workflows defined in natural language, not code
- Easy to modify - just change conductor's prompt
- Conductor can be swapped, forked, or layered (conductor of conductors)
- Worker bots stay simple - they just do tasks and write to `out`

### Conductor as Bot vs External Process

Could be:
- Another LLM bot (flexible, can adapt, but slower and uses tokens)
- A simple script that polls status and sends canned prompts (fast, deterministic)
- Hybrid: script handles timing, bot handles decisions when workflow branches

### 9p-Native Conductor

9p makes coordination trivial. Conductor just reads/writes files - no IPC, no sockets, no message queues. The filesystem *is* the coordination API.

Pseudocode:
```
while true:
    for bot in bots:
        status = read(bot/status)
        if status == "await" and bot in pending:
            next = workflow.next(bot)
            write(next.bot/in, next.prompt)
            pending.remove(bot)
            pending.add(next.bot)
    sleep(poll_interval)
```

Could be 50 lines of rc, Python, or Go. Workflow definitions could be:
- Hardcoded in the conductor
- Loaded from a config file
- Defined in a `workflow` file in 9p itself (conductor reads its own instructions from the filesystem)

## Supervisor

Separate from orchestration. Manages bot lifecycle, not workflow.

### Responsibilities

- Spawns bot processes
- Monitors for crashes
- Restarts crashed bots with same session ID (continuity)
- Possibly manages resource limits, timeouts

### Session Continuity

Key insight: if bot crashes, supervisor restarts CLI with same session ID. The 9p session state (status, out, conversation context) persists. Bot resumes where it left off, or at least the orchestrator/conductor can re-issue the last prompt.

### Relationship to Orchestrator

- `Assist` is the orchestrator (human-driven coordination via acme)
- Supervisor is a separate process that keeps bots alive
- Conductor (if implemented) handles automated workflow sequencing

Three layers, separable:
1. **Supervisor**: process health, restarts
2. **Orchestrator**: task assignment, human-driven or automated
3. **Conductor**: workflow sequencing, prompt dispatch based on status

All three interact over 9p. No special APIs - just files:
- Supervisor writes to `ctl`, reads `status` for health checks
- Orchestrator writes to `in`, reads `out`, monitors `status`
- Conductor same as orchestrator, but automated

9p is the universal bus. Components can be swapped, layered, or run on different machines (9p is network-transparent).
