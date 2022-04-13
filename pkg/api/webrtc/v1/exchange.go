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
}

func NewKick() *Kick {
	return &Kick{
		Message: Message{
			Type: TypeKick,
		},
	}
}

type Welcome struct {
	Message
	ID string `json:"id"`
}

func NewWelcome(id string) *Welcome {
	return &Welcome{
		Message: Message{
			Type: TypeWelcome,
		},
		ID: id,
	}
}
