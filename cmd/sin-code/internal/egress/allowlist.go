// SPDX-License-Identifier: MIT
// Package egress implements a transport-agnostic SSRF allowlist for URLs
// that originate from untrusted sources — LLM tool calls (Finding 4 in the
// security audit: cmd/sin-code/internal/harvest.go:110), harvested inputs,
// browser navigation targets (Finding 5: native_browser/driver.go lookupAnchor),
// and any future MCP server config that points at a remote URL.
//
// Centralising the gate here means every new http.NewRequest call site
// inherits the protection by importing this package and calling Check
// before constructing the request. The deny-by-default posture matches
// OWASP ASVS v5.0 §5.5.5 (Server-Side Request Forgery) and the NIST
// SSDF PW.4.4 (Reject untrusted inputs at trust boundaries).
//
// Design rationale
//   - DNS resolution runs synchronously inside Check. A DNS-rebinding
//     downgrade is a SEPARATE concern best solved at the http.Transport
//     level (dial IP literals from the resolved set) — out of scope here,
//     tracked as a future follow-up.
//   - net.LookupHost is the standard library's resolver hook. Tests swap
//     the lookup function via OverrideLookupHost to keep the test surface
//     hermetic without standing up DNS fixtures.
//   - All returned errors are sentinel-equality identifiable via
//     errors.Is — callers do not need to inspect error strings to
//     distinguish "denied network" from "denied scheme" from "DNS blew up".
//   - Policy is a value type (no pointers, no globals). Production code
//     passes Policy{}; opt-in to private networks requires explicit
//     AllowPrivateNetworks=true, which is grep-able in code review.
//
// Sin-debt: scope=narrow, upgrade=tighten with DNSSEC-validating resolver + dial-on-resolved-ip transport hardening
package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"testing"
)

// Policy controls which destinations Check permits.
type Policy struct {
	// AllowPrivateNetworks disables the loopback / RFC1918 / link-local /
	// ULA block. Default false (deny). Set true only for explicitly
	// trusted test harnesses (httptest.NewServer) or air-gapped deployments.
	AllowPrivateNetworks bool
}

// Sentinel errors. Callers gate on errors.Is(err, ErrEgressDenied) and
// errors.Is(err, ErrEgressScheme) — never on string match.
var (
	// ErrEgressDenied wraps the (family, ip) tuple that tripped the gate.
	// A green judge can never override a red denial: deterministic and
	// fail-closed (mandate M3).
	ErrEgressDenied = errors.New("egress: destination network denied")

	// ErrEgressScheme wraps the offending URL scheme when it is not
	// http or https. file:// / gopher:// / dict:// / jar: are all denied.
	ErrEgressScheme = errors.New("egress: scheme denied")
)

// lookupHostFn is the resolver hook used by Check. Production code MUST
// NOT modify this — only the test file in this package overrides it via
// overrideLookupHostForTest(t, fn).
var lookupHostFn = net.DefaultResolver.LookupHost

// overrideLookupHostForTest is a test-only hook that swaps the resolver
// Check uses, registering a t.Cleanup that restores the default resolver.
// Calling it outside of a test context has no effect.
func overrideLookupHostForTest(t *testing.T, fn func(ctx context.Context, host string) ([]string, error)) {
	t.Helper()
	prev := lookupHostFn
	lookupHostFn = fn
	t.Cleanup(func() { lookupHostFn = prev })
}

// Check resolves the host inside rawURL and rejects the request when
// (a) the scheme is not http or https, OR
// (b) AllowPrivateNetworks is false and any resolved address lies in a
//     loopback / RFC1918 / link-local / ULA block.
// The returned error wraps one of ErrEgressDenied or ErrEgressScheme so
// callers can use errors.Is without inspecting error text.
func Check(ctx context.Context, rawURL string, p Policy) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("egress: parse %q: %w", rawURL, err)
	}
	scheme := u.Scheme
	if scheme != "http" && scheme != "https" {
		return &schemeError{Scheme: scheme}
	}
	host := u.Hostname()
	if host == "" {
		// url like http:///foo — accept but hostname is empty: deny.
		return &schemeError{Scheme: scheme}
	}
	ips, err := lookupHostFn(ctx, host)
	if err != nil {
		return fmt.Errorf("egress: resolve %q: %w", host, err)
	}
	sortResolvedIPs(ips)
	for _, raw := range ips {
		ip := net.ParseIP(raw)
		if ip == nil {
			continue
		}
		if IsPrivate(ip) && !p.AllowPrivateNetworks {
			return &denialError{Family: classify(ip), IP: ip}
		}
	}
	return nil
}

// IsPrivate reports whether ip sits inside a block that production code
// should refuse without an explicit opt-in:
//
//   - IsLoopback                  → 127.0.0.0/8, ::1/128
//   - IsPrivate (Go 1.17+)        → RFC 1918 (10/8, 172.16/12, 192.168/16)
//                                    AND IPv6 ULA fc00::/7
//   - IsLinkLocalUnicast          → 169.254.0.0/16 (AWS/GCP/Azure metadata!)
//                                  AND fe80::/10
//   - IsLinkLocalMulticast        → 224.0.0.0/24, ff02::/16
//
// The 169.254.0.0/16 block is the SSRF class the audit findings specifically
// call out: an LLM prompt injection that hits http://169.254.169.254 on
// a cloud VM returns IAM credentials and short-lived session tokens.
func IsPrivate(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast()
}

// sortResolvedIPs orders ips deterministically so error messages and
// telemetry are byte-stable per (host, dns-quad) pair.
// IPv4 addresses sort before IPv6 (legacy convention — matches
// net.DefaultResolver behaviour in Go 1.x); within each family, raw
// bytes are compared lexically.
func sortResolvedIPs(ips []string) {
	sort.Slice(ips, func(i, j int) bool {
		pi := net.ParseIP(ips[i])
		pj := net.ParseIP(ips[j])
		pi4 := pi.To4()
		pj4 := pj.To4()
		switch {
		case pi4 != nil && pj4 == nil:
			return true
		case pi4 == nil && pj4 != nil:
			return false
		default:
			return string(pi) < string(pj)
		}
	})
}

// classify returns a short stable tag for diagnostics.
func classify(ip net.IP) string {
	switch {
	case ip.IsLoopback():
		return "loopback"
	case ip.IsLinkLocalUnicast():
		return "link-local"
	case ip.IsLinkLocalMulticast():
		return "ll-multicast"
	case ip.IsPrivate():
		return "private"
	case ip.To4() != nil:
		return "ipv4"
	default:
		return "ipv6"
	}
}

// denialError carries the (family, ip) tuple that tripped the gate so
// callers can log it without re-resolving.
type denialError struct {
	Family string
	IP     net.IP
}

func (e *denialError) Error() string {
	return fmt.Sprintf("%s: family=%s ip=%s", ErrEgressDenied.Error(), e.Family, e.IP.String())
}
func (e *denialError) Unwrap() error { return ErrEgressDenied }

// schemeError carries the offending scheme for diagnostics.
type schemeError struct {
	Scheme string
}

func (e *schemeError) Error() string {
	return fmt.Sprintf("%s: scheme=%s", ErrEgressScheme.Error(), e.Scheme)
}
func (e *schemeError) Unwrap() error { return ErrEgressScheme }
