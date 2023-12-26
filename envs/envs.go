package envs

import (
	"fmt"

	"github.com/webteleport/utils"
)

var (
	HOST     = utils.EnvHost("localhost")
	CERT     = utils.EnvCert("localhost.pem")
	KEY      = utils.EnvKey("localhost-key.pem")
	PORT     = utils.EnvPort(":3000")
	TCP_PORT = utils.EnvTCPPort(PORT)
	UDP_PORT = utils.EnvUDPPort(PORT)
	ALT_SVC  = utils.EnvAltSvc(fmt.Sprintf(`webteleport="%s"`, UDP_PORT))
)
