package v1

// Message is a generic message container
type Message struct {
	Type string `json:"type"` // Message type to unmarshal to
}
