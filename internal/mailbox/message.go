package mailbox

import (
	"encoding/json"
	"time"
)

// MessageType defines the type of message
type MessageType string

const (
	MessageTypeReviewRequest  MessageType = "REVIEW_REQUEST"
	MessageTypeReviewResponse MessageType = "REVIEW_RESPONSE"
	MessageTypeQuestion       MessageType = "QUESTION"
	MessageTypeAnswer         MessageType = "ANSWER"
	MessageTypeApprovalRequest MessageType = "APPROVAL_REQUEST"
	MessageTypeApprovalResponse MessageType = "APPROVAL_RESPONSE"
	MessageTypeStatusUpdate   MessageType = "STATUS_UPDATE"
	MessageTypeErrorReport    MessageType = "ERROR_REPORT"
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
	// Simple ID generation - reuse existing ID generation logic
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return "msg-" + string(b)
}
