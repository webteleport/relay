package manager

import (
	"expvar"
)

var WebteleportConnsRelaySpawned = expvar.NewInt("webteleport_conns_relay_spawned")
var WebteleportConnsRelayClosed = expvar.NewInt("webteleport_conns_relay_closed")
