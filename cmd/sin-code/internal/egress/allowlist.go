// SPDX-License-Identifier: MIT
// Package egress validates untrusted HTTP(S) destinations before network use.
// It rejects loopback, private, link-local, shared, multicast, unspecified, and
// benchmark ranges by default. Private-network access requires an explicit,
// grep-visible Policy opt-in.
//
// Check performs scheme, hostname, and DNS-resolution validation. That is a
// required preflight, but DNS-rebinding-resistant transports must also enforce
// the policy at dial time. The autonomy browser does this with a guarded
// DialContext and repeats validation for redirects. External engines such as
// Chrome/CDP can only use Check as defense in depth unless their resolver/dialer
// is separately pinned.
//
// Errors wrap stable sentinels so callers can distinguish denied networks,
// denied schemes, and resolver failures with errors.Is.
//
// Sin-debt: scope=narrow, upgrade=add resolver pinning for Chrome/CDP navigation
package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
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

// lookupHostFn is the resolver hook used by Check. Production code must not
// modify it; package tests replace it temporarily from allowlist_test.go.
var lookupHostFn = net.DefaultResolver.LookupHost

// ValidateURL parses rawURL and enforces scheme plus literal-host policy
// without performing DNS. It is suitable for custom/stub transports where the
// caller must avoid external resolver side effects. Real network transports
// must call Check before dialing.
func ValidateURL(rawURL string, p Policy) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("egress: parse %q: %w", rawURL, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, &schemeError{Scheme: u.Scheme}
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return nil, &schemeError{Scheme: scheme}
	}
	if p.AllowPrivateNetworks {
		return u, nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, fmt.Errorf("%w: host=%s", ErrEgressDenied, host)
	}
	if ip := net.ParseIP(host); ip != nil && IsPrivate(ip) {
		return nil, &denialError{Family: classify(ip), IP: ip}
	}
	return u, nil
}

// Check resolves the host inside rawURL and rejects the request when
// (a) the scheme is not http or https, OR
// (b) AllowPrivateNetworks is false and any resolved address lies in a
// restricted local, private, shared, multicast, or benchmark network.
//
// The returned error wraps one of ErrEgressDenied or ErrEgressScheme so
// callers can use errors.Is without inspecting error text.
func Check(ctx context.Context, rawURL string, p Policy) error {
	u, err := ValidateURL(rawURL, p)
	if err != nil {
		return err
	}
	if p.AllowPrivateNetworks {
		return nil
	}
	host := u.Hostname()
	ips, err := lookupHostFn(ctx, host)
	if err != nil {
		return fmt.Errorf("egress: resolve %q: %w", host, err)
	}
	sortResolvedIPs(ips)
	validIPs := 0
	for _, raw := range ips {
		ip := net.ParseIP(raw)
		if ip == nil {
			continue
		}
		validIPs++
		if IsPrivate(ip) {
			return &denialError{Family: classify(ip), IP: ip}
		}
	}
	if validIPs == 0 {
		return fmt.Errorf("egress: resolve %q: no usable IP addresses", host)
	}
	return nil
}

// IsPrivate reports whether ip sits inside a block that production code
// should refuse without an explicit opt-in:
//
//   - IsLoopback                  → 127.0.0.0/8, ::1/128
//   - IsPrivate (Go 1.17+)        → RFC 1918 (10/8, 172.16/12, 192.168/16)
//     AND IPv6 ULA fc00::/7
//   - IsLinkLocalUnicast          → 169.254.0.0/16 (AWS/GCP/Azure metadata!)
//     AND fe80::/10
//   - IsLinkLocalMulticast        → 224.0.0.0/24, ff02::/16
//
// The 169.254.0.0/16 block is the SSRF class the audit findings specifically
// call out: an LLM prompt injection that hits http://169.254.169.254 on
// a cloud VM returns IAM credentials and short-lived session tokens.
func IsPrivate(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 shared address space (CGNAT).
		if v4[0] == 100 && v4[1]&0xc0 == 0x40 {
			return true
		}
		// 192.0.0.0/24 protocol assignments.
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 0 {
			return true
		}
		// 198.18.0.0/15 benchmark network.
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return true
		}
	}
	return false
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
	case ip.IsMulticast():
		return "multicast"
	case ip.IsUnspecified():
		return "unspecified"
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
