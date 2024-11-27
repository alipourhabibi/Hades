package errors

var UsernameExists = New(
	"Username exists",
	AlreadyExists,
)
