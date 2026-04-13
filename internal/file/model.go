package file

// FileInfo is the internal record for a product file loaded from disk.
type FileInfo struct {
	Name      string `json:"name"`
	MimeType  string `json:"mime_type"`
	SizeBytes int    `json:"size_bytes"`
	Path      string `json:"path"`
}
