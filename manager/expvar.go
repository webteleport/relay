package manager

import (
	"expvar"
)

var (
	WebteleportStreamsRelaySpawned   = expvar.NewInt("webteleport_streams_relay_spawned")
	WebteleportStreamsRelayClosed    = expvar.NewInt("webteleport_streams_relay_closed")
	WebteleportSessionsRelayAccepted = expvar.NewInt("webteleport_sessions_relay_accepted")
	WebteleportSessionsRelayClosed   = expvar.NewInt("webteleport_sessions_relay_closed")
)
