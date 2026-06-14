package federation

import (
	"context"
	"fmt"
	"time"
)

// SPIFFEProvider implements SPIFFE identity provider
type SPIFFEProvider struct {
	trustDomain string
	privateKey  []byte
	publicKey   []byte
}

// NewSPIFFEProvider creates a new SPIFFE provider
func NewSPIFFEProvider(trustDomain string) *SPIFFEProvider {
	return &SPIFFEProvider{
		trustDomain: trustDomain,
		privateKey:  []byte("private-key"),
		publicKey:   []byte("public-key"),
	}
}

// IssueSVID issues a new SVID
func (s *SPIFFEProvider) IssueSVID(ctx context.Context, subject, namespace string) (*SVID, error) {
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}

	svid := &SVID{
		ID:          fmt.Sprintf("spiffe://%s/%s/%s", s.trustDomain, namespace, subject),
		TrustDomain: s.trustDomain,
		Subject:     subject,
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		PublicKey:   s.publicKey,
	}

	return svid, nil
}

// VerifySVID verifies an SVID
func (s *SPIFFEProvider) VerifySVID(ctx context.Context, svid *SVID) (bool, error) {
	if svid == nil {
		return false, fmt.Errorf("SVID is nil")
	}

	if svid.TrustDomain != s.trustDomain {
		return false, fmt.Errorf("invalid trust domain")
	}

	if time.Now().After(svid.ExpiresAt) {
		return false, fmt.Errorf("SVID expired")
	}

	return true, nil
}
