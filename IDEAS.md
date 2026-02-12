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
