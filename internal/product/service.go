package product

type Service struct {
	products []*Product
}

func NewService(products []*Product) *Service {
	if products == nil {
		products = []*Product{}
	}
	return &Service{products: products}
}
