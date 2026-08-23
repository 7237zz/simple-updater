package simpleupdater

type FileType string

const (
	FileTypeRegular FileType = "file"
	FileTypeSymlink FileType = "symlink"
)

type File struct {
	Path       string   `json:"path"`
	Type       FileType `json:"type,omitempty"`
	Size       uint64   `json:"size"`
	SHA256     string   `json:"sha256"`
	Mode       uint32   `json:"mode,omitempty"`
	LinkTarget string   `json:"link_target,omitempty"`
	URL        string   `json:"url"`
	Data       []byte   `json:"-"`
}

func (f File) fileType() FileType {
	if f.Type == "" {
		return FileTypeRegular
	}
	return f.Type
}
