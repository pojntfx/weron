package v1

type Greeting struct {
	Message
	ID string `json:"id"`
}

func NewGreeting(id string) *Greeting {
	return &Greeting{
		Message: Message{
			Type: TypeGreeting,
		},
		ID: id,
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
