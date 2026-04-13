package file

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// materialize writes data to {publicDir}/files/{id}/{name} if the file is not
// already present on disk. Returns the URL-relative path /files/{id}/{name}.
func (s *Service) materialize(id, name string, data []byte) (string, error) {
	diskPath := filepath.Join(s.publicDir, "files", id, name)
	if _, statErr := os.Stat(diskPath); statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return "", fmt.Errorf("stat file: %w", statErr)
		}
		dirPath := filepath.Join(s.publicDir, "files", id)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return "", fmt.Errorf("create file dir: %w", err)
		}
		if err := os.WriteFile(diskPath, data, 0644); err != nil {
			return "", fmt.Errorf("write file to disk: %w", err)
		}
	}
	return "/files/" + id + "/" + name, nil
}
