package v1

type Application struct {
	Message
	IPs []string `json:"ips"`
}

func NewApplication(ips []string) *Application {
	return &Application{
		Message: Message{
			Type: TypeApplication,
		},
		IPs: ips,
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
