package envs

import (
	"fmt"

	"github.com/btwiuse/h3/utils"
)

var (
	HOST    = utils.EnvHost("localhost")
	CERT    = utils.EnvCert("localhost.pem")
	KEY     = utils.EnvKey("localhost-key.pem")
	PORT    = utils.EnvPort(":3000")
	ALT_SVC = utils.EnvAltSvc(fmt.Sprintf(`webteleport="%s"`, PORT))
)
