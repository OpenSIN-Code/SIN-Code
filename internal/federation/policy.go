package federation

import (
	"context"
	"fmt"
	"time"
)

// PolicyEngine implements Zero-Trust policy enforcement
type PolicyEngine struct {
	policies map[string]*Policy
}

// NewPolicyEngine creates a new policy engine
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies: make(map[string]*Policy),
	}
}

// AddPolicy adds a policy to the engine
func (p *PolicyEngine) AddPolicy(policy *Policy) {
	p.policies[policy.ID] = policy
}

// Authorize makes an authorization decision
func (p *PolicyEngine) Authorize(ctx context.Context, identity *Identity, resource string, action string) (*PolicyDecision, error) {
	if identity == nil {
		return &PolicyDecision{Allow: false, Reason: "identity is nil"}, nil
	}

	applicablePolicies := p.findApplicablePolicies(resource, action)

	for _, policy := range applicablePolicies {
		if p.checkConditions(policy, identity) {
			decision := &PolicyDecision{
				Allow:  policy.Effect == "allow",
				Reason: fmt.Sprintf("matched policy %s", policy.ID),
				TTL:    1 * time.Hour,
			}
			return decision, nil
		}
	}

	return &PolicyDecision{Allow: false, Reason: "no matching policy"}, nil
}

func (p *PolicyEngine) findApplicablePolicies(resource string, action string) []*Policy {
	applicable := []*Policy{}
	for _, policy := range p.policies {
		if policy.Resource == resource && policy.Action == action {
			applicable = append(applicable, policy)
		}
	}
	return applicable
}

func (p *PolicyEngine) checkConditions(policy *Policy, identity *Identity) bool {
	if len(policy.Conditions) == 0 {
		return true
	}

	for _, condition := range policy.Conditions {
		if condition.Type == "namespace" && identity.Namespace != condition.Value {
			return false
		}
	}

	return true
}
