package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	executionSemaphore = make(chan struct{}, 3) // Max 3 concurrent executions
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
						Name:        "execute_code",
						Description: "Execute bash code as a subprocess with timeout",
						InputSchema: InputSchema{
							Type: "object",
							Properties: map[string]Property{
								"code":     {Type: "string", Description: "Bash code to execute"},
								"language": {Type: "string", Description: "Programming language (bash)", Enum: []string{"bash"}},
								"timeout":  {Type: "integer", Description: "Timeout in seconds (default: 30)"},
							},
							Required: []string{"code"},
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
	case "execute_code":
		code, _ := params.Arguments["code"].(string)
		language, _ := params.Arguments["language"].(string)
		if language == "" {
			language = "bash"
		}
		timeout := 30
		if t, ok := params.Arguments["timeout"].(float64); ok {
			timeout = int(t)
		}

		// Acquire execution slot
		executionSemaphore <- struct{}{}
		defer func() { <-executionSemaphore }()

		fmt.Fprintf(os.Stderr, "[anvilmcp] Executing bash code (timeout: %ds)\n", timeout)
		result, err := executeCode(code, language, timeout)

		// Log token comparison
		codeTokens := estimateTokens(code)
		outputTokens := estimateTokens(result)
		reduction := 0.0
		if codeTokens > 0 {
			reduction = (1.0 - float64(outputTokens)/float64(codeTokens)) * 100
		}
		logTokens(TokenLog{
			Timestamp:      time.Now(),
			Method:         "execute_code",
			DirectTokens:   codeTokens,
			CodeExecTokens: outputTokens,
			Reduction:      reduction,
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "[anvilmcp] Error: %v\n", err)
			sendError(req.ID, -32000, err.Error())
			return
		}
		fmt.Fprintf(os.Stderr, "[anvilmcp] Execution complete: %d bytes\n", len(result))
		sendResponse(req.ID, map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": result},
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

	// Parse and format the message like 9p-read-inbox does
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		// If parsing fails, return raw JSON
		return string(data), nil
	}

	// Format: [Message from {from}]\nType: {type}\nSubject: {subject}\n\n{body}
	from, _ := msg["from"].(string)
	msgType, _ := msg["type"].(string)
	subject, _ := msg["subject"].(string)
	body, _ := msg["body"].(string)

	formatted := fmt.Sprintf("[Message from %s]\nType: %s\nSubject: %s\n\n%s", from, msgType, subject, body)
	return formatted, nil
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

func listSkills() (string, error) {
	skillsPath := os.Getenv("ANVILLM_SKILLS_PATH")
	if skillsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %v", err)
		}
		skillsPath = fmt.Sprintf("%s/.config/anvillm/skills", home)
	}

	var result strings.Builder
	for _, dir := range strings.Split(skillsPath, ":") {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillPath := fmt.Sprintf("%s/%s/SKILL.md", dir, entry.Name())
			data, err := os.ReadFile(skillPath)
			if err != nil {
				continue
			}

			desc := "No description"
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "description: ") {
					desc = strings.TrimPrefix(line, "description: ")
					break
				}
			}

			result.WriteString(fmt.Sprintf("%s: %s\n", entry.Name(), desc))
		}
	}

	return result.String(), nil
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
