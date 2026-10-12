package webserver

import (
	"net"
	"net/http"
	"net/url"
)

// localOnly rejects requests that do not come from a page served by this
// server on the loopback interface. Listening on 127.0.0.1 is not enough to
// keep the photos on the machine (Docs/SPEC_trombinoscope.md §11): a
// remote web page open in the same browser could otherwise read them
// through DNS rebinding (checked via Host), or modify the trombinoscope
// through cross-site requests (checked via Origin).
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
