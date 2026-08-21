package main

type File struct {
	Path   string `json:"path"`
	Size   uint64 `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode,omitempty"`
	URL    string `json:"url"`
	Data   []byte `json:"-"`
}
