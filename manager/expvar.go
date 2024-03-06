package manager

import (
	"expvar"
)

type ExpVarStruct struct {
	WebteleportRelayStreamsSpawned   *expvar.Int
	WebteleportRelayStreamsClosed    *expvar.Int
	WebteleportRelaySessionsAccepted *expvar.Int
	WebteleportRelaySessionsClosed   *expvar.Int
}

func NewExpVarStruct() *ExpVarStruct {
	return &ExpVarStruct{
		WebteleportRelayStreamsSpawned:   expvar.NewInt("webteleport_relay_streams_spawned"),
		WebteleportRelayStreamsClosed:    expvar.NewInt("webteleport_relay_streams_closed"),
		WebteleportRelaySessionsAccepted: expvar.NewInt("webteleport_relay_sessions_accepted"),
		WebteleportRelaySessionsClosed:   expvar.NewInt("webteleport_relay_sessions_closed"),
	}
}

var expvars = NewExpVarStruct()
