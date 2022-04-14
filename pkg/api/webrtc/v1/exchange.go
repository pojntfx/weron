package v1

type Greeting struct {
	Message
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
}

func NewGreeting(id string, timestamp int64) *Greeting {
	return &Greeting{
		Message: Message{
			Type: TypeGreeting,
		},
		ID:        id,
		Timestamp: timestamp,
	}
}

type Kick struct {
	Message
	ID string `json:"id"`
}

func NewKick(id string) *Kick {
	return &Kick{
		Message: Message{
			Type: TypeKick,
		},
		ID: id,
	}
}

type Backoff struct {
	Message
}

func NewBackoff() *Backoff {
	return &Backoff{
		Message: Message{
			Type: TypeBackoff,
		},
	}
}
