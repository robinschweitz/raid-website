package v1

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func clientAddress(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
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
