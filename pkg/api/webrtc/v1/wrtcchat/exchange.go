package wrtcchat

type Application struct {
	Message
	ID string `json:"id"`
}

func NewApplication(id string) *Application {
	return &Application{
		Message: Message{
			Type: TypeApplication,
		},
		ID: id,
	}
}

type Rejection struct {
	Message
}

func NewRejection() *Rejection {
	return &Rejection{
		Message: Message{
			Type: TypeRejection,
		},
	}
}

type Acceptance struct {
	Message
	ID string `json:"id"`
}

func NewAcceptance(id string) *Acceptance {
	return &Acceptance{
		Message: Message{
			Type: TypeAcceptance,
		},
		ID: id,
	}
}
