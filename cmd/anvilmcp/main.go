package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
						Description: "Execute bash via one of three mutually exclusive modes (pipe wins if present):\n1. tool+args — run a named library tool: {tool: 'list_sessions.sh', args: ['foo']}\n2. code+args — run inline bash: {code: 'echo hello'}\n3. pipe — Unix pipeline of stages, stdout→stdin: {pipe: [{tool: 'list_sessions.sh'}, {code: 'grep active'}, {tool: 'format_table.sh', args: ['--compact']}]}\nEach pipe stage is {tool?, args?, code?}. All-tool pipes are trusted (skip validation); any inline code stage triggers validation.",
						InputSchema: InputSchema{
							Type: "object",
							Properties: map[string]Property{
								"tool":     {Type: "string", Description: "Mode 1: named tool from anvillm/tools/ (e.g. 'list_sessions.sh')"},
								"args":     {Type: "array", Description: "Positional args ($1 $2 …) for the tool or code", Items: &Items{Type: "string"}},
								"code":     {Type: "string", Description: "Mode 2: inline bash to execute directly"},
								"language": {Type: "string", Description: "Programming language", Enum: []string{"bash"}},
								"timeout":  {Type: "integer", Description: "Timeout in seconds (default: 30). Recommended: 600 for builds, 120 for network ops, 1800 for remote builds"},
								"sandbox":  {Type: "string", Description: "Sandbox config name (default: anvilmcp)"},
								"pipe":     {Type: "array", Description: "Mode 3: ordered pipeline stages, each {tool?, args?, code?}. Stages run as a single bash -c; stdout of each feeds stdin of the next.", Items: &Items{Type: "object"}},
							},
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
		tool, _ := params.Arguments["tool"].(string)
		var toolArgs []string
		if argsRaw, ok := params.Arguments["args"].([]interface{}); ok {
			for _, a := range argsRaw {
				if s, ok := a.(string); ok {
					toolArgs = append(toolArgs, s)
				}
			}
		} else if argsStr, ok := params.Arguments["args"].(string); ok {
			// LLM passed args as a JSON-encoded string instead of an array
			var parsed []string
			if err := json.Unmarshal([]byte(argsStr), &parsed); err == nil {
				toolArgs = parsed
			}
		}
		code, _ := params.Arguments["code"].(string)
		language, _ := params.Arguments["language"].(string)

		// Parse pipe steps
		var pipeSteps []PipeStep
		if pipeRaw, ok := params.Arguments["pipe"].([]interface{}); ok {
			for _, stepRaw := range pipeRaw {
				stepMap, ok := stepRaw.(map[string]interface{})
				if !ok {
					continue
				}
				step := PipeStep{}
				step.Tool, _ = stepMap["tool"].(string)
				step.Code, _ = stepMap["code"].(string)
				if argsRaw, ok := stepMap["args"].([]interface{}); ok {
					for _, a := range argsRaw {
						if s, ok := a.(string); ok {
							step.Args = append(step.Args, s)
						}
					}
				} else if argsStr, ok := stepMap["args"].(string); ok {
					var parsed []string
					if err := json.Unmarshal([]byte(argsStr), &parsed); err == nil {
						step.Args = parsed
					}
				}
				pipeSteps = append(pipeSteps, step)
			}
		}

		// Resolve execution mode: pipe > tool > inline code
		trusted := false
		if len(pipeSteps) > 0 {
			var err error
			code, trusted, err = buildPipeline(pipeSteps)
			if err != nil {
				sendError(req.ID, -32000, err.Error())
				return
			}
		} else if tool != "" {
			toolCode, err := readTool(tool)
			if err != nil {
				sendError(req.ID, -32000, fmt.Sprintf("failed to read tool %s: %v", tool, err))
				return
			}
			code = toolCode
			trusted = true
			// For bash with args, wrap script to receive positional params
			if len(toolArgs) > 0 {
				// Escape args for bash using single quotes (prevents all expansion)
				var escaped []string
				for _, arg := range toolArgs {
					// Replace ' with '\'' (end quote, escaped quote, start quote)
					escaped = append(escaped, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
				}
				code = fmt.Sprintf("set -- %s\n%s", strings.Join(escaped, " "), code)
			}
		}

		if code == "" {
			sendError(req.ID, -32602, "either 'tool', 'code', or 'pipe' is required")
			return
		}

		if language == "" {
			language = "bash"
		}
		timeout := 30
		if t, ok := params.Arguments["timeout"].(float64); ok {
			timeout = int(t)
		}
		sandbox, _ := params.Arguments["sandbox"].(string)
		sandboxExplicit := sandbox != ""
		if sandbox == "" {
			sandbox = "anvilmcp"
		}

		// Acquire execution slot
		executionSemaphore <- struct{}{}
		defer func() { <-executionSemaphore }()
		fmt.Fprintf(os.Stderr, "[anvilmcp] Executing %s code (timeout: %ds, sandbox: %s, trusted: %v)\n", language, timeout, sandbox, trusted)
		result, err := executeCode(code, language, timeout, sandbox, trusted)

		// Fallback to default sandbox on permission errors (only if sandbox wasn't explicit)
		if err != nil && !sandboxExplicit && isPermissionError(err) {
			fmt.Fprintf(os.Stderr, "[anvilmcp] Permission error in %s sandbox, falling back to default\n", sandbox)
			result, err = executeCode(code, language, timeout, "default", trusted)
		}

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
