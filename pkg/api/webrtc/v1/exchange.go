package v1

type Greeting struct {
	Message
	IDs       map[string]struct{} `json:"ids"`
	Timestamp int64               `json:"timestamp"`
}

func NewGreeting(id map[string]struct{}, timestamp int64) *Greeting {
	return &Greeting{
		Message: Message{
			Type: TypeGreeting,
		},
		IDs:       id,
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
