package backend

import (
	"errors"
	"log/slog"
	"net/http/httputil"
	"net/url"

	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
)

// New returns a backend server of type `t`.
func New(t base.LBType, u *url.URL, rp *httputil.ReverseProxy, logger *slog.Logger) (Server, error) {
	switch t {
	case base.PromLB:
		return NewTSDB(u, rp, logger), nil
	case base.PyroLB:
		return NewPyroscope(u, rp, logger), nil
	}

	return nil, errors.New("unknown load balancer type. Only tsdb and pyroscope types supported")
}
