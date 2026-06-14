package federation

import (
	"time"
)

// SVID represents a SPIFFE Verifiable Identity
type SVID struct {
	ID        string    `json:"id"`
	TrustDomain string  `json:"trust_domain"`
	Subject   string    `json:"subject"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	PublicKey []byte    `json:"public_key"`
}

// Identity represents a federated identity
type Identity struct {
	SVID      *SVID
	Principal string
	Namespace string
	Attributes map[string]string
}

// Policy represents a Zero-Trust policy
type Policy struct {
	ID        string
	Name      string
	Resource  string
	Action    string
	Effect    string // allow, deny
	Conditions []Condition
}

// Condition represents a policy condition
type Condition struct {
	Type  string
	Value string
}

// PolicyDecision represents an authorization decision
type PolicyDecision struct {
	Allow   bool
	Reason  string
	TTL     time.Duration
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        string
	Timestamp time.Time
	Identity  *Identity
	Action    string
	Resource  string
	Result    string
	Details   map[string]interface{}
}
