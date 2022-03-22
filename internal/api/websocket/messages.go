package websocket

type Message struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Payload []byte `json:"payload"`
}

func NewRequest(id string) *Message {
	return &Message{
		Type: TypeRequest,
		ID:   id,
	}
}

func NewOffer(id string, payload []byte) *Message {
	return &Message{
		Type: TypeOffer,
		ID:   id,
	}
}

func NewAnswer(id string, payload []byte) *Message {
	return &Message{
		Type: TypeAnswer,
		ID:   id,
	}
}

func NewCandidate(id string, payload []byte) *Message {
	return &Message{
		Type: TypeCandidate,
		ID:   id,
	}
}
