# Code Execution Example Workflows

Example workflows demonstrating the code execution pattern with bash scripts.

## 1. Inbox Processing

Process all messages in inbox and summarize:

```bash
#!/bin/bash
# Process inbox messages

agent_id="$1"
count=0

while true; do
  msg=$(./check_inbox.sh 2>&1)
  if [[ "$msg" == "No messages" ]]; then
    break
  fi
  count=$((count + 1))
done

echo "Processed $count messages"
```

## 2. Multi-Agent Status Check

Check status of all active agents:

```bash
#!/bin/bash
# Check all agent states

sessions=$(./list_sessions.sh)
total=$(echo "$sessions" | jq 'length')
running=$(echo "$sessions" | jq '[.[] | select(.state == "running")] | length')
idle=$(echo "$sessions" | jq '[.[] | select(.state == "idle")] | length')

echo "Total agents: $total"
echo "Running: $running"
echo "Idle: $idle"
```

## 3. Polling Loop

Wait for specific condition:

```bash
#!/bin/bash
# Poll inbox until specific message arrives

agent_id="$1"
keyword="$2"
attempts=0
max_attempts=10

while [ $attempts -lt $max_attempts ]; do
  msg=$(./check_inbox.sh 2>&1)
  
  if echo "$msg" | grep -q "$keyword"; then
    echo "Found message with keyword: $keyword"
    exit 0
  fi
  
  if [[ "$msg" == "No messages" ]]; then
    sleep 5
  fi
  
  attempts=$((attempts + 1))
done

echo "Timeout: keyword not found after $max_attempts attempts"
exit 1
```

## 4. Broadcast Message

Send message to all active agents:

```bash
#!/bin/bash
# Broadcast message to all agents

from="$1"
subject="$2"
body="$3"

sessions=$(./list_sessions.sh)
recipients=$(echo "$sessions" | jq -r '.[].id')

for to in $recipients; do
  if [ "$to" != "$from" ]; then
    ./send_message.sh "$from" "$to" "NOTIFICATION" "$subject" "$body"
    echo "Sent to $to"
  fi
done
```

## 5. Agent Health Monitor

Monitor agent states and alert on errors:

```bash
#!/bin/bash
# Monitor agent health

while true; do
  sessions=$(./list_sessions.sh)
  errors=$(echo "$sessions" | jq -r '.[] | select(.state == "error") | .id')
  
  if [ -n "$errors" ]; then
    for agent_id in $errors; do
      echo "ALERT: Agent $agent_id in error state"
      ./send_message.sh "monitor" "user" "ALERT" \
        "Agent Error" "Agent $agent_id is in error state"
    done
  fi
  
  sleep 30
done
```

## 6. Skill Discovery

List and filter available skills:

```bash
#!/bin/bash
# Find skills matching keyword

keyword="$1"
skills=$(./list_skills.sh)

matches=$(echo "$skills" | jq -r \
  --arg kw "$keyword" \
  '.[] | select(.description | contains($kw)) | .name')

if [ -z "$matches" ]; then
  echo "No skills found matching: $keyword"
else
  echo "Skills matching '$keyword':"
  echo "$matches"
fi
```

## 7. Message Routing

Route messages based on content:

```bash
#!/bin/bash
# Route messages to appropriate handlers

agent_id="$1"

while true; do
  msg=$(./check_inbox.sh 2>&1)
  
  if [[ "$msg" == "No messages" ]]; then
    sleep 5
    continue
  fi
  
  type=$(echo "$msg" | grep "^Type:" | cut -d: -f2 | xargs)
  
  case "$type" in
    PROMPT_REQUEST)
      echo "Routing to prompt handler"
      ;;
    REVIEW_REQUEST)
      echo "Routing to review handler"
      ;;
    QUERY_REQUEST)
      echo "Routing to query handler"
      ;;
    *)
      echo "Unknown message type: $type"
      ;;
  esac
done
```

## 8. Batch State Updates

Update multiple agent states:

```bash
#!/bin/bash
# Set all idle agents to running

sessions=$(./list_sessions.sh)
idle_agents=$(echo "$sessions" | jq -r '.[] | select(.state == "idle") | .id')

for agent_id in $idle_agents; do
  ./set_state.sh "$agent_id" "running"
  echo "Set $agent_id to running"
done
```

## 9. Data Aggregation

Aggregate data from multiple sources:

```bash
#!/bin/bash
# Collect stats from all agents

sessions=$(./list_sessions.sh)

echo "Agent Statistics:"
echo "================="
echo "$sessions" | jq -r '.[] | "\(.id): \(.state) in \(.workdir)"'

echo ""
echo "State Summary:"
echo "$sessions" | jq -r 'group_by(.state) | .[] | "\(.[0].state): \(length)"'
```

## 10. Privacy-Preserving Pipeline

Process sensitive data without exposing it:

```bash
#!/bin/bash
# Process messages without logging content

agent_id="$1"
processed=0
errors=0

while true; do
  msg=$(./check_inbox.sh 2>&1)
  
  if [[ "$msg" == "No messages" ]]; then
    break
  fi
  
  # Process message (content never logged)
  if echo "$msg" | grep -q "Type:"; then
    processed=$((processed + 1))
  else
    errors=$((errors + 1))
  fi
done

# Only log counts, not content
echo "Processed: $processed, Errors: $errors"
```

## Token Savings

These workflows demonstrate token efficiency:

**Without code execution:**
- Each tool call: ~90 tokens (schema)
- Each result: full data in context
- 10 tool calls: ~900 tokens + data

**With code execution:**
- Tool discovery: ~30 tokens
- Code execution: ~200 tokens
- Result: summary only (~20 tokens)
- Total: ~250 tokens (72% reduction)

## Best Practices

1. **Filter early**: Process data in bash, return summaries
2. **Use loops**: Avoid repeated tool calls through context
3. **Handle errors**: Check exit codes and output
4. **Log summaries**: Never log full sensitive data
5. **Timeout protection**: Use max attempts in loops
