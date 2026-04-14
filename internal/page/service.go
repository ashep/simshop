package page

import "context"

// Service holds the in-memory list of pages loaded at startup.
type Service struct {
	pages []*Page
}

func NewService(pages []*Page) *Service {
	if pages == nil {
		pages = []*Page{}
	}
	return &Service{pages: pages}
}

func (s *Service) List(_ context.Context) ([]*Page, error) {
	return s.pages, nil
}
