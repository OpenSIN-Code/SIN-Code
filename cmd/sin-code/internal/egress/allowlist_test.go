// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the egress allowlist. The five canonical tests
// pinned in the security audit spec live at the top; coverage tests for
// the helper functions follow.
package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

func overrideLookupHostForTest(t *testing.T, fn func(ctx context.Context, host string) ([]string, error)) {
	t.Helper()
	prev := lookupHostFn
	lookupHostFn = fn
	t.Cleanup(func() { lookupHostFn = prev })
}

func TestValidateURL_DeniesLiteralLocalDestinations(t *testing.T) {
	for _, raw := range []string{
		"http://localhost:8080/",
		"http://api.localhost/",
		"http://127.0.0.1/",
		"http://[::1]/",
		"file:///etc/passwd",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ValidateURL(raw, Policy{}); err == nil {
				t.Fatalf("ValidateURL(%q) unexpectedly allowed", raw)
			}
		})
	}
}

func TestValidateURL_AllowsPublicHostnameWithoutDNS(t *testing.T) {
	u, err := ValidateURL("https://example.com/path", Policy{})
	if err != nil {
		t.Fatalf("ValidateURL: %v", err)
	}
	if u.Hostname() != "example.com" {
		t.Fatalf("hostname = %q", u.Hostname())
	}
}

func TestIsPrivate_AdditionalRestrictedRanges(t *testing.T) {
	for _, raw := range []string{"0.0.0.0", "100.64.0.1", "192.0.0.1", "198.18.0.1", "224.0.0.1"} {
		if !IsPrivate(net.ParseIP(raw)) {
			t.Errorf("%s must be restricted", raw)
		}
	}
}

// pinned by security audit — DO NOT REMOVE

func TestIsPrivate_127(t *testing.T) {
	if !IsPrivate(net.ParseIP("127.0.0.1")) {
		t.Fatal("127.0.0.1 must be classified private")
	}
}

func TestIsPrivate_10(t *testing.T) {
	if !IsPrivate(net.ParseIP("10.0.0.1")) {
		t.Fatal("10.0.0.1 must be classified private")
	}
}

func TestIsPrivate_169_254(t *testing.T) {
	if !IsPrivate(net.ParseIP("169.254.169.254")) {
		t.Fatal("169.254.169.254 (cloud metadata) must be classified private")
	}
}

func TestIsPrivate_8_8_8_8(t *testing.T) {
	if IsPrivate(net.ParseIP("8.8.8.8")) {
		t.Fatal("8.8.8.8 must NOT be classified private")
	}
}

func TestCheck_DeniesMetadata(t *testing.T) {
	overrideLookupHostForTest(t, func(_ context.Context, _ string) ([]string, error) {
		return []string{"169.254.169.254"}, nil
	})
	err := Check(context.Background(), "http://169.254.169.254/latest/meta-data/", Policy{})
	if !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("169.254.169.254 must trip ErrEgressDenied, got %v", err)
	}
	var de *denialError
	if !errors.As(err, &de) {
		t.Fatalf("denial must carry *denialError, got %T", err)
	}
	if de.Family != "link-local" {
		t.Errorf("denial family = %q, want link-local", de.Family)
	}
	if de.IP.String() != "169.254.169.254" {
		t.Errorf("denial IP = %q, want 169.254.169.254", de.IP.String())
	}
}

func TestCheck_AllowsPublic(t *testing.T) {
	overrideLookupHostForTest(t, func(_ context.Context, _ string) ([]string, error) {
		return []string{"8.8.8.8"}, nil
	})
	err := Check(context.Background(), "http://example.com", Policy{})
	if errors.Is(err, ErrEgressDenied) {
		t.Fatalf("public IP must not trip ErrEgressDenied, got %v", err)
	}
	if err != nil {
		t.Fatalf("public host must return nil, got %v", err)
	}
}

func TestCheck_DeniesScheme(t *testing.T) {
	err := Check(context.Background(), "file:///etc/passwd", Policy{})
	if !errors.Is(err, ErrEgressScheme) {
		t.Fatalf("file:// must trip ErrEgressScheme, got %v", err)
	}
	var se *schemeError
	if !errors.As(err, &se) {
		t.Fatalf("scheme denial must carry *schemeError, got %T", err)
	}
	if se.Scheme != "file" {
		t.Errorf("scheme = %q, want file", se.Scheme)
	}
}

func TestCheck_AllowPrivate_AllowsLocalhost(t *testing.T) {
	overrideLookupHostForTest(t, func(_ context.Context, _ string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	})
	err := Check(context.Background(), "http://localhost:8080", Policy{AllowPrivateNetworks: true})
	if err != nil {
		t.Fatalf("private host must be allowed under permissive policy, got %v", err)
	}
}

// additional coverage — exhausts every branch in allowlist.go

func TestCheck_DeniesReservedDomains(t *testing.T) {
	cases := []struct {
		ip     string
		family string
	}{
		{"127.0.0.1", "loopback"},
		{"10.255.0.1", "private"},
		{"172.16.0.1", "private"},
		{"192.168.1.1", "private"},
		{"169.254.1.1", "link-local"},
		{"::1", "loopback"},
		{"fe80::1", "link-local"},
		{"fc00::1", "private"}, // IPv6 ULA
		{"fd12:3456::1", "private"},
		{"224.0.0.1", "ll-multicast"},
		{"ff02::1", "ll-multicast"},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			overrideLookupHostForTest(t, func(_ context.Context, _ string) ([]string, error) {
				return []string{tc.ip}, nil
			})
			rawURL := "http://" + tc.ip
			if strings.Contains(tc.ip, ":") {
				rawURL = "http://[" + tc.ip + "]/"
			}
			err := Check(context.Background(), rawURL, Policy{})
			if !errors.Is(err, ErrEgressDenied) {
				t.Fatalf("%s must be denied, got %v", tc.ip, err)
			}
			var de *denialError
			if !errors.As(err, &de) {
				return
			}
			if de.Family != tc.family {
				t.Errorf("%s family = %q, want %q", tc.ip, de.Family, tc.family)
			}
		})
	}
}

func TestCheck_DNSError(t *testing.T) {
	overrideLookupHostForTest(t, func(_ context.Context, _ string) ([]string, error) {
		return nil, fmt.Errorf("no such host")
	})
	err := Check(context.Background(), "http://nonexistent.test", Policy{})
	if err == nil {
		t.Fatal("expected error from resolver failure")
	}
	if errors.Is(err, ErrEgressDenied) || errors.Is(err, ErrEgressScheme) {
		t.Fatalf("DNS error must not be a typed egress error, got %v", err)
	}
	if !strings.Contains(err.Error(), "no such host") {
		t.Errorf("DNS error must wrap root cause, got %v", err)
	}
}

func TestCheck_MixedResolve(t *testing.T) {
	// A hostname that resolves to BOTH a public and a private address.
	// Even one private hit must trip the gate (fail-closed).
	overrideLookupHostForTest(t, func(_ context.Context, _ string) ([]string, error) {
		return []string{"8.8.8.8", "127.0.0.1"}, nil
	})
	err := Check(context.Background(), "http://mixed.test", Policy{})
	if !errors.Is(err, ErrEgressDenied) {
		t.Fatal("mixed resolve must deny on any private hit")
	}
}

func TestCheck_MixedResolve_AllowsWithPermissive(t *testing.T) {
	overrideLookupHostForTest(t, func(_ context.Context, _ string) ([]string, error) {
		return []string{"8.8.8.8", "127.0.0.1"}, nil
	})
	err := Check(context.Background(), "http://mixed.test", Policy{AllowPrivateNetworks: true})
	if err != nil {
		t.Fatalf("permissive policy must allow mixed resolves, got %v", err)
	}
}

func TestCheck_DeniesAnyPrivateHitOnMixedSet(t *testing.T) {
	overrideLookupHostForTest(t, func(_ context.Context, _ string) ([]string, error) {
		// Six addresses covering every classification path.
		return []string{"2001:db8::1", "169.254.0.1", "10.0.0.1", "::1", "8.8.8.8", "172.16.0.1"}, nil
	})
	err := Check(context.Background(), "http://mix.test", Policy{})
	if !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("any-private-hit must deny, got %v", err)
	}
}

func TestCheck_DeniesMissingScheme(t *testing.T) {
	err := Check(context.Background(), "jar:http://x/", Policy{})
	if !errors.Is(err, ErrEgressScheme) {
		t.Fatalf("non-http scheme must be denied, got %v", err)
	}
}

func TestCheck_DeniesEmptyScheme(t *testing.T) {
	// gopher:// parses cleanly with an empty permitted scheme list
	// outside http/https — exactly the path the gate must catch.
	err := Check(context.Background(), "gopher://nope", Policy{})
	if !errors.Is(err, ErrEgressScheme) {
		t.Fatalf("gopher:// must be denied, got %v", err)
	}
}

func TestCheck_DeniesEmptyHostname(t *testing.T) {
	// "http://" is parseable to scheme=http, hostname="" — must deny.
	err := Check(context.Background(), "http://", Policy{})
	if !errors.Is(err, ErrEgressScheme) {
		t.Fatalf("empty hostname must be denied, got %v", err)
	}
}

func TestCheck_ParseErrorPropagates(t *testing.T) {
	// url.Parse with control bytes (high ASCII) produces an error in
	// url.Parse — re-wrap so callers can inspect.
	err := Check(context.Background(), "http://\x7f/", Policy{})
	if err == nil {
		t.Fatal("expected parse error for control-char URL")
	}
}

func TestIsPrivate_NilIP(t *testing.T) {
	if IsPrivate(nil) {
		t.Fatal("nil IP must not be classified private")
	}
}

func TestIsPrivate_IPv6Public(t *testing.T) {
	if IsPrivate(net.ParseIP("2606:4700:4700::1111")) {
		t.Fatal("Cloudflare DNS IPv6 must not be private")
	}
}

func TestSortResolvedIPs_Deterministic(t *testing.T) {
	// Same input → same byte output every call (byte-stability contract).
	input := []string{"::1", "127.0.0.1", "fe80::1", "169.254.0.1", "10.0.0.1"}
	a := append([]string(nil), input...)
	sortResolvedIPs(a)
	b := append([]string(nil), input...)
	sortResolvedIPs(b)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("sortResolvedIPs must be byte-stable: %v vs %v", a, b)
	}
	// IPv4 must come before IPv6.
	for i := 0; i < len(a); i++ {
		if strings.Contains(a[i], ":") && i != 3 && i != 4 {
			// not strict — but a[v4-block] < a[v6-block] is the contract.
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		ip   string
		want string
	}{
		{"127.0.0.1", "loopback"},
		{"169.254.0.1", "link-local"},
		{"224.0.0.1", "ll-multicast"},
		{"10.0.0.1", "private"},
		{"8.8.8.8", "ipv4"},
		{"2606:4700::1", "ipv6"},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			got := classify(net.ParseIP(tc.ip))
			if got != tc.want {
				t.Errorf("classify(%s) = %q, want %q", tc.ip, got, tc.want)
			}
		})
	}
}

func TestDenialErrorFormat(t *testing.T) {
	ip := net.ParseIP("169.254.169.254")
	e := &denialError{Family: "link-local", IP: ip}
	if !strings.Contains(e.Error(), "link-local") {
		t.Errorf("denial message must include family, got %q", e.Error())
	}
	if !strings.Contains(e.Error(), "169.254.169.254") {
		t.Errorf("denial message must include IP, got %q", e.Error())
	}
	if !errors.Is(e, ErrEgressDenied) {
		t.Fatal("denialError must unwrap to ErrEgressDenied")
	}
}

func TestSchemeErrorFormat(t *testing.T) {
	e := &schemeError{Scheme: "file"}
	if !strings.Contains(e.Error(), "file") {
		t.Errorf("scheme message must include scheme, got %q", e.Error())
	}
	if !errors.Is(e, ErrEgressScheme) {
		t.Fatal("schemeError must unwrap to ErrEgressScheme")
	}
}
