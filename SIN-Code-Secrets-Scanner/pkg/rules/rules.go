// SPDX-License-Identifier: MIT
package rules

import (
	"github.com/OpenSIN-Code/SIN-Code-Secrets-Scanner/pkg/models"
)

// AllRules returns all built-in secret detection rules
func AllRules() []models.DetectionRule {
	return []models.DetectionRule{
		// ── API Keys ────────────────────────────────────────
		{
			ID:          "SECRETS-001",
			Name:        "OpenAI API Key",
			SecretType:  "api-key",
			Severity:    "critical",
			Patterns:    []string{"sk-[a-zA-Z0-9]{20,48}", "sk-proj-[a-zA-Z0-9]{20,}"},
			Remediation: "Remove from code. Use environment variables or a secret manager (e.g., AWS Secrets Manager, HashiCorp Vault).",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-002",
			Name:        "AWS Access Key ID",
			SecretType:  "api-key",
			Severity:    "critical",
			Patterns:    []string{"AKIA[0-9A-Z]{16}", "ASIA[0-9A-Z]{16}"},
			Remediation: "Rotate the key immediately. Use IAM roles instead of long-term credentials. Store in AWS Secrets Manager.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-003",
			Name:        "AWS Secret Access Key",
			SecretType:  "api-key",
			Severity:    "critical",
			Patterns:    []string{"[0-9a-zA-Z/+]{40}", "aws_secret_access_key[\\s]*[=:][\\s]*['\"]?[0-9a-zA-Z/+]{40}"},
			Remediation: "Rotate the key immediately. Never commit AWS Secret Access Keys. Use IAM roles.",
			Confidence:  "medium",
			MinEntropy:  4.0,
		},
		{
			ID:          "SECRETS-004",
			Name:        "GitHub Personal Access Token",
			SecretType:  "api-key",
			Severity:    "critical",
			Patterns:    []string{"ghp_[a-zA-Z0-9]{36}", "gho_[a-zA-Z0-9]{36}", "ghu_[a-zA-Z0-9]{36}", "ghs_[a-zA-Z0-9]{36}", "ghr_[a-zA-Z0-9]{36}"},
			Remediation: "Revoke the token in GitHub Settings. Use GitHub Apps or OAuth Apps instead of PATs. Store in environment variables.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-005",
			Name:        "Slack Token",
			SecretType:  "api-key",
			Severity:    "high",
			Patterns:    []string{"xox[baprs]-[0-9]{10,13}-[0-9]{10,13}-[a-zA-Z0-9]{24}", "xox[baprs]-[0-9a-zA-Z]{10,48}"},
			Remediation: "Revoke the token in Slack Admin. Use Slack Apps with restricted scopes. Store in environment variables.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-006",
			Name:        "Stripe API Key",
			SecretType:  "api-key",
			Severity:    "critical",
			Patterns:    []string{"sk_live_[0-9a-zA-Z]{24,}", "pk_live_[0-9a-zA-Z]{24,}", "sk_test_[0-9a-zA-Z]{24,}", "pk_test_[0-9a-zA-Z]{24,}"},
			Remediation: "Roll the key in Stripe Dashboard. Use Stripe Connect or restricted keys. Never commit live keys.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-007",
			Name:        "Google API Key",
			SecretType:  "api-key",
			Severity:    "high",
			Patterns:    []string{"AIza[0-9A-Za-z_-]{35}", "google_api_key[\\s]*[=:][\\s]*['\"]?[A-Za-z0-9_-]{35,}"},
			Remediation: "Restrict the key in Google Cloud Console. Use Application Default Credentials instead. Store in environment variables.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-008",
			Name:        "Twilio API Key",
			SecretType:  "api-key",
			Severity:    "high",
			Patterns:    []string{"SK[0-9a-f]{32}", "twilio_api_key[\\s]*[=:][\\s]*['\"]?SK[0-9a-f]{32}"},
			Remediation: "Revoke the key in Twilio Console. Use subaccounts with restricted permissions. Store in environment variables.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-009",
			Name:        "SendGrid API Key",
			SecretType:  "api-key",
			Severity:    "high",
			Patterns:    []string{"SG\\.[a-zA-Z0-9_-]{22}\\.[a-zA-Z0-9_-]{43}", "sendgrid_api_key[\\s]*[=:][\\s]*['\"]?SG\\."},
			Remediation: "Revoke the key in SendGrid Dashboard. Use API key scopes to restrict permissions. Store in environment variables.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-010",
			Name:        "Mailgun API Key",
			SecretType:  "api-key",
			Severity:    "high",
			Patterns:    []string{"key-[0-9a-zA-Z]{32}", "mailgun_api_key[\\s]*[=:][\\s]*['\"]?key-[0-9a-zA-Z]{32}"},
			Remediation: "Rotate the key in Mailgun Dashboard. Use domain-specific keys. Store in environment variables.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
		// ── Authentication Tokens ────────────────────────────
		{
			ID:          "SECRETS-011",
			Name:        "JWT Token",
			SecretType:  "token",
			Severity:    "high",
			Patterns:    []string{"eyJ[a-zA-Z0-9_-]*\\.eyJ[a-zA-Z0-9_-]*\\.[a-zA-Z0-9_-]*", "bearer[\\s]+[a-zA-Z0-9_-]{20,}"},
			Remediation: "Never commit JWTs. Use short-lived tokens with refresh token rotation. Store in secure HTTP-only cookies.",
			Confidence:  "medium",
			MinEntropy:  3.5,
		},
		{
			ID:          "SECRETS-012",
			Name:        "Bearer Token / OAuth Token",
			SecretType:  "token",
			Severity:    "high",
			Patterns:    []string{"Authorization:[\\s]*Bearer[\\s]+[a-zA-Z0-9_-]{20,}", "access_token[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9_-]{20,}"},
			Remediation: "Never commit bearer tokens. Use OAuth 2.0 with PKCE. Store tokens in secure session storage.",
			Confidence:  "medium",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-013",
			Name:        "Generic API Key",
			SecretType:  "api-key",
			Severity:    "medium",
			Patterns:    []string{"api[_-]?key[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9]{16,48}", "apikey[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9]{16,48}", "api_key[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9]{16,48}"},
			Remediation: "Move API keys to environment variables or a secret manager. Use key rotation policies.",
			Confidence:  "low",
			MinEntropy:  3.0,
		},
		// ── Passwords & Credentials ──────────────────────────
		{
			ID:          "SECRETS-014",
			Name:        "Database Password",
			SecretType:  "password",
			Severity:    "critical",
			Patterns:    []string{"password[\\s]*[=:][\\s]*['\"][^'\"]{4,}", "passwd[\\s]*[=:][\\s]*['\"][^'\"]{4,}", "db_password[\\s]*[=:][\\s]*['\"][^'\"]{4,}", "database_password[\\s]*[=:][\\s]*['\"][^'\"]{4,}"},
			Remediation: "Use environment variables or a secret manager. Rotate passwords regularly. Use connection string builders.",
			Confidence:  "medium",
			MinEntropy:  2.0,
		},
		{
			ID:          "SECRETS-015",
			Name:        "Private Key (RSA/ECDSA/ED25519)",
			SecretType:  "private-key",
			Severity:    "critical",
			Patterns:    []string{"-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----", "-----BEGIN ENCRYPTED PRIVATE KEY-----"},
			Remediation: "Never commit private keys. Use SSH agent or hardware security modules (HSM). Store in secure key management.",
			Confidence:  "high",
			MinEntropy:  4.0,
		},
		{
			ID:          "SECRETS-016",
			Name:        "Certificate / PEM",
			SecretType:  "certificate",
			Severity:    "high",
			Patterns:    []string{"-----BEGIN CERTIFICATE-----", "-----BEGIN RSA PRIVATE KEY-----"},
			Remediation: "Store certificates in secure vaults or HSM. Use certificate rotation. Never commit .pem files.",
			Confidence:  "high",
			MinEntropy:  4.0,
		},
		// ── Configuration Files ──────────────────────────────
		{
			ID:          "SECRETS-017",
			Name:        ".env File with Secrets",
			SecretType:  "config-file",
			Severity:    "high",
			Patterns:    []string{"\\.env", "\\.env\\.local", "\\.env\\.production"},
			Remediation: "Add .env files to .gitignore. Use a secret manager for production. Never commit .env files.",
			Confidence:  "low",
			MinEntropy:  0.0,
		},
		{
			ID:          "SECRETS-018",
			Name:        "Docker Config / Registry Auth",
			SecretType:  "config-file",
			Severity:    "high",
			Patterns:    []string{"\\.docker/config\\.json", "auth[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9+/]{20,}=="},
			Remediation: "Use Docker credential helpers. Store registry credentials in a secret manager. Never commit Docker config.",
			Confidence:  "medium",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-019",
			Name:        "Kubernetes Secret",
			SecretType:  "config-file",
			Severity:    "critical",
			Patterns:    []string{"kind:[\\s]*Secret", "secretName:[\\s]*[a-zA-Z0-9-]+", "data:[\\s]*\\n[\\s]*[a-zA-Z0-9_-]+:[\\s]*[a-zA-Z0-9+/]{20,}"},
			Remediation: "Use external secret operators (e.g., External Secrets Operator). Encrypt secrets at rest. Never commit raw secrets.",
			Confidence:  "medium",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-020",
			Name:        "Terraform Cloud Token / State",
			SecretType:  "token",
			Severity:    "critical",
			Patterns:    []string{"terraform\\.cloud[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9_-]{20,}", "TF_API_TOKEN[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9_-]{20,}", "terraform\\.tfstate"},
			Remediation: "Use remote state with encryption (S3 with SSE-KMS). Use Terraform Cloud workspaces. Never commit .tfstate files.",
			Confidence:  "medium",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-021",
			Name:        "Heroku API Key",
			SecretType:  "api-key",
			Severity:    "high",
			Patterns:    []string{"heroku_api_key[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9-]{20,}", "HEROKU_API_KEY[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9-]{20,}"},
			Remediation: "Rotate the key in Heroku Dashboard. Use Heroku Config Vars. Store in environment variables.",
			Confidence:  "medium",
			MinEntropy:  3.0,
		},
		{
			ID:          "SECRETS-022",
			Name:        "Discord Webhook / Bot Token",
			SecretType:  "token",
			Severity:    "high",
			Patterns:    []string{"https://discord\\.com/api/webhooks/[0-9]{18}/[a-zA-Z0-9_-]{68}", "discord_bot_token[\\s]*[=:][\\s]*['\"]?[a-zA-Z0-9_-]{20,}"},
			Remediation: "Regenerate the webhook/token in Discord Developer Portal. Use environment variables. Restrict bot permissions.",
			Confidence:  "high",
			MinEntropy:  3.0,
		},
	}
}

// FilterRulesByType returns rules for specific secret types
func FilterRulesByType(rules []models.DetectionRule, types []string) []models.DetectionRule {
	if len(types) == 0 {
		return rules
	}
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}
	var filtered []models.DetectionRule
	for _, r := range rules {
		if typeSet[r.SecretType] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// FilterRulesBySeverity returns rules with severity >= given threshold
func FilterRulesBySeverity(rules []models.DetectionRule, minSeverity string) []models.DetectionRule {
	severityOrder := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	min := severityOrder[minSeverity]
	if min == 0 {
		min = 1
	}
	var filtered []models.DetectionRule
	for _, r := range rules {
		if severityOrder[r.Severity] >= min {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
