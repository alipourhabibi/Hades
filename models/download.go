package models

type DownloadResponseContent struct {
	Commit *Commit `json:"commit,omitempty"`
	Files  []*File `json:"files,omitempty"`
}

type DownloadResponse struct {
	Contents []*DownloadResponseContent `json:"contents,omitempty"`
}
