package relay

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"

	"github.com/btwiuse/dispatcher"
	"github.com/btwiuse/muxr"
	"github.com/btwiuse/tags"
	"github.com/webteleport/utils"
	"github.com/webteleport/webteleport/edge"
)

var _ Ingress = (*IngressHandler)(nil)

var DefaultIngress = NewIngressHandler(DefaultStorage)

type IngressHandler struct {
	*muxr.Router
	storage Storage
}

func NewIngress() Ingress {
	s := NewStore()
	return NewIngressHandler(s)
}

func NewIngressHandler(storage Storage) *IngressHandler {
	i := &IngressHandler{
		Router:  muxr.NewRouter(),
		storage: storage,
	}
	i.Router.Handle("/", dispatcher.DispatcherFunc(i.Dispatch))
	return i
}

func (i *IngressHandler) Use(middlewares ...muxr.Middleware) {
	i.Router.Use(middlewares...)
}

func (i *IngressHandler) GetRoundTripper(h string) (http.RoundTripper, bool) {
	tssn, ok := i.storage.GetSession(h)
	if !ok {
		return nil, false
	}
	return RoundTripper(tssn), true
}

func (i *IngressHandler) RecordsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	all := i.storage.Records()
	filtered := []*Record{}
	for _, rec := range all {
		if rec.Matches(r.URL.Query()) {
			filtered = append(filtered, rec)
		}
	}
	resp, err := tags.UnescapedJSONMarshalIndent(filtered, "  ")
	if err != nil {
		slog.Warn(fmt.Sprintf("json marshal failed: %s", err))
		return
	}
	w.Write(resp)
}

func (i *IngressHandler) Dispatch(r *http.Request) http.Handler {
	rt, ok := i.GetRoundTripper(r.Host)
	if !ok {
		return utils.HostNotFoundHandler()
	}
	rp := utils.LoggedReverseProxy(rt)
	rp.Rewrite = func(req *httputil.ProxyRequest) {
		req.SetXForwarded()
		req.Out.URL.Host = r.Host
		req.Out.URL.Scheme = "http"
	}
	rp.ModifyResponse = func(resp *http.Response) error {
		i.storage.Visited(r.Host)
		expvars.WebteleportRelayStreamsClosed.Add(1)
		return nil
	}
	return rp
}

func (i *IngressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	i.Router.ServeHTTP(w, r)
}

func (i *IngressHandler) Subscribe(upgrader edge.Upgrader) {
	i.storage.Subscribe(upgrader)
}
