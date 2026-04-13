package product

import "context"

func (s *Service) GetPrice(_ context.Context, id string, countryID string) (*PriceResult, error) {
	p, ok := s.index[id]
	if !ok {
		return nil, ErrProductNotFound
	}
	if v, ok := p.Prices[countryID]; ok {
		return &PriceResult{CountryID: countryID, Value: v}, nil
	}
	if v, ok := p.Prices["DEFAULT"]; ok {
		return &PriceResult{CountryID: "DEFAULT", Value: v}, nil
	}
	return &PriceResult{CountryID: "DEFAULT", Value: 0}, nil
}
