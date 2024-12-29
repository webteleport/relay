package relay

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"

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
	rec, ok := i.storage.GetRecord(h)
	if !ok {
		return nil, false
	}
	return rec.RoundTripper, true
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

func (i *IngressHandler) AliasHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		i.getAliases(w, r)
	case http.MethodPost, http.MethodPut:
		i.setAlias(w, r)
	case http.MethodDelete:
		i.deleteAlias(w, r)
	}
}

func (i *IngressHandler) getAliases(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	all := i.storage.Aliases()
	resp, err := tags.UnescapedJSONMarshalIndent(all, "  ")
	if err != nil {
		slog.Warn(fmt.Sprintf("json marshal failed: %s", err))
		return
	}
	w.Write(resp)
}

// example curl request:
// curl -X POST -d "<alias> <target>" http://localhost:8080/alias
func (i *IngressHandler) setAlias(w http.ResponseWriter, r *http.Request) {
	// get request body as string and split it into alias and target
	b, err := io.ReadAll(r.Body)
	if err == nil {
		line := strings.TrimSpace(string(b))
		parts := strings.Split(line, " ")
		if len(parts) == 2 {
			alias, target := parts[0], parts[1]
			i.storage.Alias(alias, target)
		}
	}
}

// example curl request:
// curl -X DELETE -d "<alias>" http://localhost:8080/alias
func (i *IngressHandler) deleteAlias(w http.ResponseWriter, r *http.Request) {
	// get request body as string and split it into alias
	b, err := io.ReadAll(r.Body)
	if err == nil {
		alias := strings.TrimSpace(string(b))
		i.storage.Unalias(alias)
	}
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
