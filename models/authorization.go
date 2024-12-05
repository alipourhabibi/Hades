package models

type Subject string

const (
	OWNER Subject = "owner"
)

type Object string

const (
	REPOSITORY Object = "repository"
)

type Action string

const (
	CREATE Action = "create"
)

type CanResponse struct {
	Allowed bool
}

type Policy struct {
	Subject string
	Domain  string
	Object  string
	Action  string
}

type Role struct {
	User   string
	Role   string
	Domain string
}
