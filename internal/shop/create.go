package shop

type CreateRequest struct {
	ID string `json:"id"`
}

func (s *Service) Create(req CreateRequest) (*Shop, error) {
	res := &Shop{
		ID:    req.ID,
		Names: nil,
	}

	return res, nil
}
