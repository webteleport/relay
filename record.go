package relay

import (
	"net/url"
	"slices"
	"time"

	"github.com/btwiuse/tags"
	"github.com/webteleport/webteleport/tunnel"
)

type Record struct {
	Key     string         `json:"key"`
	Session tunnel.Session `json:"-"`
	Header  tags.Tags      `json:"header"`
	Tags    tags.Tags      `json:"tags"`
	Since   time.Time      `json:"since"`
	Visited int            `json:"visited"`
	IP      string         `json:"ip"`
	Path    string         `json:"path"`
}

func (r *Record) Matches(kvs url.Values) (ok bool) {
	for k, v := range kvs {
		// r.Tags contains k
		tv, has := r.Tags.Values[k]
		if !has {
			return false
		}
		// tv is superset of v
		for _, vv := range v {
			if !slices.Contains(tv, vv) {
				return false
			}
		}
	}
	return true
}
