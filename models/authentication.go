package models

type SigninRequest struct {
	Username    string
	Password    string
	Description string
}

type SigninResponse struct {
	User *User
}
