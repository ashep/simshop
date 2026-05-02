package shop

import (
	"context"
	"sort"
)

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

// Name returns the shop name in the requested language, falling back to the
// alphabetically first available language if lang is missing. Returns "" if
// the shop has no name at all.
func (s *Service) Name(lang string) string {
	if s == nil || s.shop == nil {
		return ""
	}
	if v := s.shop.Name[lang]; v != "" {
		return v
	}
	keys := make([]string, 0, len(s.shop.Name))
	for k := range s.shop.Name {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return s.shop.Name[keys[0]]
}
