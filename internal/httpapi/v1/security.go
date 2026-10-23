package v1

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// clientAddress returns the address used for per-client limits such as the
// login rate limit. Behind the trusted reverse proxy, X-Real-IP is preferred
// because nginx overwrites it with the connecting address. X-Forwarded-For is
// only a fallback, and then only its last entry: nginx appends the real address
// to whatever the client sent, so earlier entries are chosen by the client.
func clientAddress(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func webAuthnRelyingParty(publicOrigin string, r *http.Request, trustProxyHeaders bool) (rpID, origin string, ok bool) {
	if publicOrigin != "" {
		u, err := url.Parse(publicOrigin)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "", "", false
		}
		host := u.Hostname()
		if host == "" {
			return "", "", false
		}
		return host, u.Scheme + "://" + u.Host, true
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if trustProxyHeaders {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
			scheme = strings.ToLower(strings.Split(forwarded, ",")[0])
		}
	}
	host := r.Host
	if trustProxyHeaders {
		if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
			host = strings.Split(forwardedHost, ",")[0]
		}
	}
	if host == "" {
		return "", "", false
	}
	rpHost := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		rpHost = parsedHost
	}
	return rpHost, scheme + "://" + host, true
}
