package errors

func FromCasbin(err error) error {
	return New("Internal", Internal)
}
