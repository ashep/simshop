package file

type Service struct {
	files map[string][]FileInfo // product ID → files
}

func NewService(files map[string][]FileInfo) *Service {
	if files == nil {
		files = make(map[string][]FileInfo)
	}
	return &Service{files: files}
}
