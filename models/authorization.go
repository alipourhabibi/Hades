package models

type CanResponse struct {
	Allowed bool
}

type Policy struct {
	Subject string
	Object  string
	Action  string
}
