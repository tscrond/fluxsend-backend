package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// TrustedProxyClientIP reads X-Forwarded-For only from configured proxy peers.
func TrustedProxyClientIP(trustedProxyCIDRs []string) func(http.Handler) http.Handler {
	prefixes := make([]netip.Prefix, 0, len(trustedProxyCIDRs))
	canonicalTrustedProxyCIDRs := make([]string, 0, len(trustedProxyCIDRs))
	for _, rawPrefix := range trustedProxyCIDRs {
		prefix, err := parseTrustedProxyPrefix(rawPrefix)
		if err != nil {
			panic(fmt.Sprintf("invalid trusted proxy CIDR %q: %v", rawPrefix, err))
		}
		prefixes = append(prefixes, prefix)
		canonicalTrustedProxyCIDRs = append(canonicalTrustedProxyCIDRs, prefix.String())
	}

	direct := chimiddleware.ClientIPFromRemoteAddr
	xff := chimiddleware.ClientIPFromXFF(canonicalTrustedProxyCIDRs...)

	return func(next http.Handler) http.Handler {
		directNext := direct(next)
		forwardedNext := direct(xff(next))

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, ok := parseRemoteIP(r.RemoteAddr)
			if ok && inAnyPrefix(peer, prefixes) {
				forwardedNext.ServeHTTP(w, r)
				return
			}
			directNext.ServeHTTP(w, r)
		})
	}
}

func parseRemoteIP(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	if addr.Zone() != "" {
		addr = addr.WithZone("")
	}
	return addr.Unmap(), true
}

func parseTrustedProxyPrefix(raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Prefix{}, fmt.Errorf("empty value")
	}

	if strings.Contains(raw, "/") {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, err
		}
		if prefix.Addr().Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("zones are not supported")
		}
		return prefix.Masked(), nil
	}

	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, err
	}
	if addr.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("zones are not supported")
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

func inAnyPrefix(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
