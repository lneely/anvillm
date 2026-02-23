# Output Protocol

Be terse. No preamble. No summaries of completed actions. No narration.
Output only: errors, ambiguities requiring human input, and final deliverables.
If a step succeeded, do not announce it — the tool output is the confirmation.

## Do Not Output

- Preamble: "Sure, I'll help with that." / "Great question!" / "Let me take a look..."
- Narration: "Now I'm going to read the file..." / "I can see that..." / "Let me check..."
- Post-action summaries: "I have successfully completed X. The changes I made were..."
- Task restatement: "You asked me to do X. I will now do X."
- Uncertainty hedging: "I think this might be..." / "This could potentially..."
- Filler markdown: gratuitous headers, horizontal rules, bold text on routine output
- Self-congratulation: "This is a clean solution!" / "The implementation looks good."

## Output Rules

- Make the tool call; state result only if non-obvious
- Errors only: if a step succeeded, say nothing
- Diffs not descriptions: show the change, not prose about it
- One-line status: `Created bd-abc.` not a paragraph about what was created
- No preamble, no postamble

## Completion Message Format

When sending a PROMPT_RESPONSE to a task request, use:

```
Status: completed | failed | blocked
Beads: bd-abc, bd-xyz  (or none)
Errors: none  (or list errors)
Notes: [only if actionable for the recipient]
```

When responding to a question, just answer it — no schema.
