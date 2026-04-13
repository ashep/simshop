package product

type Service struct {
	products []*Product
	index    map[string]*Product
}

func NewService(products []*Product) *Service {
	if products == nil {
		products = []*Product{}
	}
	index := make(map[string]*Product, len(products))
	for _, p := range products {
		index[p.ID] = p
	}
	return &Service{products: products, index: index}
}
