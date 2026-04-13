package product

import "context"

func (s *Service) Get(_ context.Context, id string) (*Product, error) {
	p, ok := s.index[id]
	if !ok {
		return nil, ErrProductNotFound
	}
	return p, nil
}
