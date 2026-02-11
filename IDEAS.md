# IDEAS.md

Vision: An LLM orchestrator that automates tasks with LLMs using Acme as the interface.

## Fire and Forget Prompting

Type a multi-line prompt, middle-click (X-x event) on a session ID to pass the prompt to that tmux session. Use case: start a task without following output (e.g., "build the program, fix errors and retry until successful").

Implementation sketch:
- Acme event handler watches for X-x (B2 chord) on session IDs
- Extract selected text from current window as the prompt
- Send prompt to the target session's tmux pane via `tmux send-keys`

## Shared Context

Inject context from one session into another.

Possible approaches:
- Export conversation summary to a file, import into another session
- Shared 9p file that multiple sessions can read from
- Command to "copy context" between sessions via the orchestrator
- Named context snippets that can be referenced by any session
