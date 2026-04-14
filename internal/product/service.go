package product

type Service struct {
	items []*Item
}

func NewService(items []*Item) *Service {
	if items == nil {
		items = []*Item{}
	}
	return &Service{items: items}
}
