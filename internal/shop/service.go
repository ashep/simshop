package shop

import "context"

// Service holds the in-memory shop data loaded at startup.
type Service struct {
	shop *Shop
}

func NewService(s *Shop) *Service {
	if s == nil {
		s = &Shop{}
	}
	return &Service{shop: s}
}

func (s *Service) Get(_ context.Context) (*Shop, error) {
	return s.shop, nil
}
