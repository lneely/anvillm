package mailbox

import (
	"encoding/json"
	"fmt"
	"time"
)

// MessageType defines the type of message
type MessageType string

const (
	// Ephemeral message types (log only, auto-complete)
	MessageTypeLogInfo  MessageType = "LOG_INFO"  // Status updates, progress
	MessageTypeLogError MessageType = "LOG_ERROR" // Errors

	// Persistent message types (inbox only)
	MessageTypePrompt           MessageType = "PROMPT"            // User instructions to bot
	MessageTypeQueryRequest     MessageType = "QUERY_REQUEST"     // Request information
	MessageTypeQueryResponse    MessageType = "QUERY_RESPONSE"    // Provide information
	MessageTypeReviewRequest    MessageType = "REVIEW_REQUEST"    // Request code review
	MessageTypeReviewResponse   MessageType = "REVIEW_RESPONSE"   // Provide review feedback
	MessageTypeApprovalRequest  MessageType = "APPROVAL_REQUEST"  // Request testing/approval
	MessageTypeApprovalResponse MessageType = "APPROVAL_RESPONSE" // Provide test results

	// Deprecated types (for backward compatibility)
	MessageTypeQuestion     MessageType = "QUESTION"      // Deprecated: use QUERY_REQUEST
	MessageTypeAnswer       MessageType = "ANSWER"        // Deprecated: use QUERY_RESPONSE
	MessageTypeStatusUpdate MessageType = "STATUS_UPDATE" // Deprecated: use LOG_INFO
	MessageTypeErrorReport  MessageType = "ERROR_REPORT"  // Deprecated: use LOG_ERROR
)

// Message represents a structured message between sessions
type Message struct {
	ID        string                 `json:"id"`
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	Type      MessageType            `json:"type"`
	Subject   string                 `json:"subject"`
	Body      string                 `json:"body"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp int64                  `json:"timestamp"`
	Retries   int                    `json:"retries"`
}

// NewMessage creates a new message with generated ID and timestamp
func NewMessage(from, to string, msgType MessageType, subject, body string) *Message {
	return &Message{
		ID:        generateID(),
		From:      from,
		To:        to,
		Type:      msgType,
		Subject:   subject,
		Body:      body,
		Metadata:  make(map[string]interface{}),
		Timestamp: time.Now().Unix(),
	}
}

// ToJSON serializes the message to JSON
func (m *Message) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// FromJSON deserializes a message from JSON
func FromJSON(data []byte) (*Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return &msg, err
}

func generateID() string {
	return fmt.Sprintf("msg-%d", time.Now().Unix())
}

// ValidateMessageType checks if the message type is valid and returns an error if deprecated or invalid
func ValidateMessageType(msgType MessageType) error {
	switch msgType {
	// Valid ephemeral types
	case MessageTypeLogInfo, MessageTypeLogError:
		return nil
	// Valid persistent types
	case MessageTypePrompt, MessageTypeQueryRequest, MessageTypeQueryResponse,
		MessageTypeReviewRequest, MessageTypeReviewResponse,
		MessageTypeApprovalRequest, MessageTypeApprovalResponse:
		return nil
	// Deprecated types
	case MessageTypeQuestion:
		return fmt.Errorf("deprecated type: use QUERY_REQUEST instead")
	case MessageTypeAnswer:
		return fmt.Errorf("deprecated type: use QUERY_RESPONSE instead")
	case MessageTypeStatusUpdate:
		return fmt.Errorf("deprecated type: use LOG_INFO instead")
	case MessageTypeErrorReport:
		return fmt.Errorf("deprecated type: use LOG_ERROR instead")
	default:
		return fmt.Errorf("invalid message type. valid types are: LOG_INFO, LOG_ERROR, PROMPT, QUERY_REQUEST, QUERY_RESPONSE, REVIEW_REQUEST, REVIEW_RESPONSE, APPROVAL_REQUEST, APPROVAL_RESPONSE")
	}
}
