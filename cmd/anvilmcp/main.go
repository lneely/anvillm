package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Items       *Items   `json:"items,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type Items struct {
	Type string `json:"type"`
}

func main() {
	fmt.Fprintln(os.Stderr, "[anvilmcp] Starting MCP server")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		fmt.Fprintf(os.Stderr, "[anvilmcp] Received: %s\n", string(line))
		var req MCPRequest
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintf(os.Stderr, "[anvilmcp] Parse error: %v\n", err)
			sendError(nil, -32700, "Parse error")
			continue
		}

		fmt.Fprintf(os.Stderr, "[anvilmcp] Method: %s\n", req.Method)
		switch req.Method {
		case "initialize":
			sendResponse(req.ID, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]bool{},
				},
				"serverInfo": map[string]string{
					"name":    "anvilmcp",
					"version": "0.1.0",
				},
			})
		case "notifications/initialized":
			// Notification - no response needed
			fmt.Fprintln(os.Stderr, "[anvilmcp] Initialized notification received")
		case "tools/list":
			sendResponse(req.ID, map[string]interface{}{
				"tools": []Tool{
					{
						Name:        "read_inbox",
						Description: "Read messages from agent's inbox",
						InputSchema: InputSchema{
							Type: "object",
							Properties: map[string]Property{
								"agent_id": {Type: "string", Description: "Agent session ID (or 'user')"},
							},
							Required: []string{"agent_id"},
						},
					},
					{
						Name:        "send_message",
						Description: "Send message to another agent or user",
						InputSchema: InputSchema{
							Type: "object",
							Properties: map[string]Property{
								"from":    {Type: "string", Description: "Sender agent ID (or 'user')"},
								"to":      {Type: "string", Description: "Recipient agent ID (or 'user')"},
								"type":    {Type: "string", Description: "Message type"},
								"subject": {Type: "string", Description: "Message subject"},
								"body":    {Type: "string", Description: "Message body"},
							},
							Required: []string{"from", "to", "type", "subject", "body"},
						},
					},
					{
						Name:        "list_sessions",
						Description: "List all active sessions",
						InputSchema: InputSchema{
							Type:       "object",
							Properties: map[string]Property{},
						},
					},
					{
						Name:        "set_state",
						Description: "Set agent state (idle, running, stopped, etc.)",
						InputSchema: InputSchema{
							Type: "object",
							Properties: map[string]Property{
								"agent_id": {Type: "string", Description: "Agent session ID"},
								"state":    {Type: "string", Description: "State value", Enum: []string{"idle", "running", "stopped", "starting", "error", "exited"}},
							},
							Required: []string{"agent_id", "state"},
						},
					},
				},
			})
		case "tools/call":
			handleToolCall(req)
		default:
			sendError(req.ID, -32601, "Method not found")
		}
	}
}

func handleToolCall(req MCPRequest) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		fmt.Fprintf(os.Stderr, "[anvilmcp] Invalid params: %v\n", err)
		sendError(req.ID, -32602, "Invalid params")
		return
	}

	fmt.Fprintf(os.Stderr, "[anvilmcp] Tool call: %s with args: %v\n", params.Name, params.Arguments)
	switch params.Name {
	case "read_inbox":
		agentID, _ := params.Arguments["agent_id"].(string)
		fmt.Fprintf(os.Stderr, "[anvilmcp] Reading inbox for: %s\n", agentID)
		result, err := readInbox(agentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[anvilmcp] Error: %v\n", err)
			sendError(req.ID, -32000, err.Error())
			return
		}
		fmt.Fprintf(os.Stderr, "[anvilmcp] Success: %d bytes\n", len(result))
		sendResponse(req.ID, map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": result},
			},
		})
	case "send_message":
		from, _ := params.Arguments["from"].(string)
		to, _ := params.Arguments["to"].(string)
		msgType, _ := params.Arguments["type"].(string)
		subject, _ := params.Arguments["subject"].(string)
		body, _ := params.Arguments["body"].(string)
		
		fmt.Fprintf(os.Stderr, "[anvilmcp] Sending message: %s -> %s (type: %s)\n", from, to, msgType)
		err := sendMessage(from, to, msgType, subject, body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[anvilmcp] Error: %v\n", err)
			sendError(req.ID, -32000, err.Error())
			return
		}
		fmt.Fprintf(os.Stderr, "[anvilmcp] Message sent successfully\n")
		sendResponse(req.ID, map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": "Message sent"},
			},
		})
	case "list_sessions":
		fmt.Fprintf(os.Stderr, "[anvilmcp] Listing sessions\n")
		result, err := listSessions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[anvilmcp] Error: %v\n", err)
			sendError(req.ID, -32000, err.Error())
			return
		}
		fmt.Fprintf(os.Stderr, "[anvilmcp] Found sessions: %d bytes\n", len(result))
		sendResponse(req.ID, map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": result},
			},
		})
	case "set_state":
		agentID, _ := params.Arguments["agent_id"].(string)
		state, _ := params.Arguments["state"].(string)
		
		fmt.Fprintf(os.Stderr, "[anvilmcp] Setting state for %s to %s\n", agentID, state)
		err := setState(agentID, state)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[anvilmcp] Error: %v\n", err)
			sendError(req.ID, -32000, err.Error())
			return
		}
		fmt.Fprintf(os.Stderr, "[anvilmcp] State set successfully\n")
		sendResponse(req.ID, map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": "State set"},
			},
		})
	default:
		sendError(req.ID, -32601, "Tool not found")
	}
}

func readInbox(agentID string) (string, error) {
	inboxPath := fmt.Sprintf("agent/%s/inbox", agentID)
	
	cmd := exec.Command("9p", "ls", inboxPath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list inbox: %v", err)
	}
	
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 0 || files[0] == "" {
		return "No messages", nil
	}
	
	// Read first message only
	var firstFile string
	for _, file := range files {
		if strings.HasSuffix(file, ".json") {
			firstFile = file
			break
		}
	}
	
	if firstFile == "" {
		return "No messages", nil
	}
	
	msgPath := fmt.Sprintf("%s/%s", inboxPath, firstFile)
	cmd = exec.Command("9p", "read", msgPath)
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read message: %v", err)
	}
	
	// Mark as complete (strip .json extension)
	msgID := strings.TrimSuffix(firstFile, ".json")
	ctlPath := fmt.Sprintf("agent/%s/ctl", agentID)
	cmd = exec.Command("9p", "write", ctlPath)
	cmd.Stdin = strings.NewReader(fmt.Sprintf("complete %s", msgID))
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[anvilmcp] Warning: Failed to mark message as complete: %v\n", err)
	}
	
	return string(data), nil
}

func sendMessage(from, to, msgType, subject, body string) error {
	msg := map[string]string{
		"to":      to,
		"type":    msgType,
		"subject": subject,
		"body":    body,
	}
	
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	
	mailPath := fmt.Sprintf("agent/%s/mail", from)
	cmd := exec.Command("9p", "write", mailPath)
	cmd.Stdin = strings.NewReader(string(data))
	
	// Capture both stdout and stderr to get error messages
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Return the actual error message from 9p
		if len(output) > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(string(output)))
		}
		return err
	}
	return nil
}

func listSessions() (string, error) {
	cmd := exec.Command("9p", "read", "agent/list")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	return string(output), nil
}

func setState(agentID, state string) error {
	statePath := fmt.Sprintf("agent/%s/state", agentID)
	cmd := exec.Command("9p", "write", statePath)
	cmd.Stdin = strings.NewReader(state)
	return cmd.Run()
}

func sendResponse(id interface{}, result interface{}) {
	resp := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
	os.Stdout.Sync()
}

func sendError(id interface{}, code int, message string) {
	resp := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &MCPError{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
	os.Stdout.Sync()
}
