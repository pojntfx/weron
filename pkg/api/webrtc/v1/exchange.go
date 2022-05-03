package v1

// Greeting is a claim for a set of IDs
type Greeting struct {
	Message
	IDs       map[string]struct{} `json:"ids"`       // IDs to claim one of
	Timestamp int64               `json:"timestamp"` // Timestamp to resolve conflicts
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

// Kick notifies peers that an ID has already been claimed
type Kick struct {
	Message
	ID string `json:"id"` // ID which has already been claimed
}

func NewKick(id string) *Kick {
	return &Kick{
		Message: Message{
			Type: TypeKick,
		},
		ID: id,
	}
}

// Backoff asks a peer to back off from claiming IDs
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

// Claimed notifies a peer that an ID has already been claimed
type Claimed struct {
	Message
	ID string `json:"id"` // ID which has already been claimed
}

func NewClaimed(id string) *Claimed {
	return &Claimed{
		Message: Message{
			Type: TypeClaimed,
		},
		ID: id,
	}
}
