package websocket

type Message struct {
	Type string `json:"type"`
}

type Introduction struct {
	*Message

	From string `json:"from"`
}

type Exchange struct {
	*Message

	From    string `json:"from"`
	To      string `json:"to"`
	Payload []byte `json:"payload"`
}

func NewIntroduction(from string) *Introduction {
	return &Introduction{
		Message: &Message{
			Type: TypeIntroduction,
		},
		From: from,
	}
}

func NewOffer(from string, to string, payload []byte) *Exchange {
	return &Exchange{
		Message: &Message{
			Type: TypeOffer,
		},
		From:    from,
		To:      to,
		Payload: payload,
	}
}

func NewAnswer(from string, to string, payload []byte) *Exchange {
	return &Exchange{
		Message: &Message{
			Type: TypeAnswer,
		},
		From:    from,
		To:      to,
		Payload: payload,
	}
}

func NewCandidate(from string, to string, payload []byte) *Exchange {
	return &Exchange{
		Message: &Message{
			Type: TypeCandidate,
		},
		From:    from,
		To:      to,
		Payload: payload,
	}
}
