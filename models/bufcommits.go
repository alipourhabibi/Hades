package models

type ResourceRefs struct {
	Id        string `json:"id"`
	Owner     string `json:"owner,omitempty"`
	Module    string `json:"module,omitempty"`
	LabelName string `json:"label_name,omitempty"`
	Ref       string `json:"ref,omitempty"`
}
