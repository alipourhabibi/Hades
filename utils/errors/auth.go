package grpc

import (
	pkgerr "github.com/alipourhabibi/Hades/internal/pkg/errors"
)

func MapUserAuthError(err error) error {
	pkgErr := pkgerr.FromGorm(err).(pkgerr.PkgError)
	if pkgErr.Code == pkgerr.NotFound {
		pkgErr.Code = pkgerr.Unauthenticated
		pkgErr.Message = "Unauthenticated"
	}
	return pkgErr
}
