package property

type Service struct {
	props []Property
}

func NewService(props []Property) *Service {
	if props == nil {
		props = []Property{}
	}
	return &Service{props: props}
}
