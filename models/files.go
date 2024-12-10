package models

type File struct {
	Path    string `json:"path,omitempty"`
	Content []byte `json:"content,omitempty"`
}
