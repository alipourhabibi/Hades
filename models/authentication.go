package models

type SigninRequest struct {
	Username    string
	Password    string
	Description string
}

type SigninResponse struct {
	User *User
}

type LoginRequest struct {
	Username string
	Password string
}

type LoginResponse struct {
	Token string
}
