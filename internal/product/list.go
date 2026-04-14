package product

import "context"

func (s *Service) List(_ context.Context) ([]*Item, error) {
	return s.items, nil
}
