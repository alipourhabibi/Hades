package gitaly

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
)

func gitalyAddr(c config.Gitaly) string {
	host := c.Host
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s:%d", host, c.Port)
}
