// Package clientip resolves the client IP address of a request, honouring
// forwarding headers only when the immediate peer is a configured trusted
// proxy.
//
// Forwarding headers are trivially forged by any client. Every consumer of the
// client IP in this codebase is security-relevant (login, registration and
// password-reset rate limiting, plus the IP recorded in the audit log), so an
// unconditional read of X-Forwarded-For lets an attacker rotate the header per
// request to defeat all rate limiting and to forge the IP attributed to any
// audited event.
package clientip

import (
	"net"
	"net/netip"
	"strings"
)

// TrustedProxies is a parsed list of CIDR ranges whose forwarding headers are
// honoured. A nil or empty value means no proxy is trusted, so the peer address
// is always used.
type TrustedProxies []netip.Prefix

// ParseTrustedProxies parses CIDR strings into a TrustedProxies set.
// A bare IP address is accepted and treated as a single-host range.
func ParseTrustedProxies(cidrs []string) (TrustedProxies, error) {
	out := make(TrustedProxies, 0, len(cidrs))
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if p, err := netip.ParsePrefix(raw); err == nil {
			out = append(out, p)
			continue
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// contains reports whether addr falls inside any trusted range.
func (t TrustedProxies) contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, p := range t {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// FromHeaders returns the client IP for a request.
//
// peerAddr is the transport-level remote address ("host:port" or a bare host).
// xff and xRealIP are the raw X-Forwarded-For and X-Real-IP header values.
//
// When the peer is not a trusted proxy the peer address is returned and the
// headers are ignored entirely. When it is trusted, the X-Forwarded-For chain
// is walked from the right, skipping hops that are themselves trusted, and the
// first untrusted address is returned: that is the closest hop the trusted
// infrastructure actually observed, and the last one an attacker cannot forge.
func FromHeaders(peerAddr, xff, xRealIP string, trusted TrustedProxies) string {
	peer := hostOnly(peerAddr)
	if len(trusted) == 0 {
		return peer
	}
	peerAddrParsed, err := netip.ParseAddr(peer)
	if err != nil || !trusted.contains(peerAddrParsed) {
		return peer
	}

	if xff != "" {
		hops := strings.Split(xff, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			hop := strings.TrimSpace(hops[i])
			addr, err := netip.ParseAddr(hostOnly(hop))
			if err != nil {
				continue
			}
			if trusted.contains(addr) {
				continue
			}
			return addr.String()
		}
	}
	if xRealIP != "" {
		if addr, err := netip.ParseAddr(hostOnly(strings.TrimSpace(xRealIP))); err == nil {
			return addr.String()
		}
	}
	return peer
}

// hostOnly strips an optional ":port" suffix from addr.
func hostOnly(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	// Bracketed IPv6 without a port, e.g. "[::1]".
	return strings.Trim(addr, "[]")
}
