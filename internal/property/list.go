package property

import "context"

func (s *Service) List(_ context.Context) ([]Property, error) {
	return s.props, nil
}
