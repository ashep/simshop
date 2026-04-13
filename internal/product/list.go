package product

import "context"

func (s *Service) List(_ context.Context) ([]*Product, error) {
	return s.products, nil
}
