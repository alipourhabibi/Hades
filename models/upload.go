package models

type UploadRequest struct {
	Contents     []*UploadRequest_Content `json:"contents,omitempty"`
	DepCommitIds []string                 `json:"dep_commit_ids,omitempty"`
}

type UploadRequest_Content struct {
	ModuleRef        *ModuleRef `json:"module_ref,omitempty"`
	Files            []*File    `json:"files,omitempty"`
	SourceControlUrl string     `json:"source_control_url,omitempty"`
	// ScopedLabelRefs []*ScopedLabelRef `json:"scoped_label_refs,omitempty"` // TODO do we need it
}
