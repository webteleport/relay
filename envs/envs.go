package envs

import (
	"fmt"
	"os"

	"github.com/webteleport/utils"
)

var (
	HOST       = utils.EnvHost("localhost")
	CERT       = utils.EnvCert("localhost.pem")
	KEY        = utils.EnvKey("localhost-key.pem")
	PORT       = utils.EnvPort(":3000")
	UDP_PORT   = utils.EnvUDPPort(PORT)
	HTTP_PORT  = LookupEnvPort("HTTP_PORT")
	HTTPS_PORT = LookupEnvPort("HTTPS_PORT")

	ALT_SVC = utils.EnvAltSvc(fmt.Sprintf(`webteleport="%s"`, UDP_PORT))
)

func LookupEnv(key string) *string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	return &v
}

func LookupEnvPort(key string) *string {
	v := LookupEnv(key)
	if v == nil {
		return nil
	}
	p := ":" + *v
	return &p
}
