package file

import "context"

func (s *Service) GetForProduct(_ context.Context, productID string) ([]FileInfo, error) {
	files := s.files[productID]
	if files == nil {
		return []FileInfo{}, nil
	}
	return files, nil
}
