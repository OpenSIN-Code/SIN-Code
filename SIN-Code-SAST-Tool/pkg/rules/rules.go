package rules

import (
	"github.com/OpenSIN-Code/SIN-Code-SAST-Tool/pkg/models"
)

// AllRules returns all built-in SAST detection rules
func AllRules() []models.Rule {
	return []models.Rule{
		// ── SQL Injection ──────────────────────────────────────
		{
			ID:          "SAST-001",
			Name:        "SQL Injection (Raw Query)",
			Description: "User input concatenated directly into SQL query",
			Severity:    "critical",
			CWE:         "CWE-89",
			OWASP:       "A03:2021 - Injection",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php"},
			Patterns:    []string{"execute(\\s*\\(", "query(\\s*\\(", "exec(\\s*\\("},
			Remediation: "Use parameterized queries/prepared statements instead of string concatenation",
			Confidence:  "high",
			Category:    "injection",
		},
		{
			ID:          "SAST-002",
			Name:        "SQL Injection (String Formatting)",
			Description: "SQL query constructed with string formatting (f-string, % format, .format)",
			Severity:    "critical",
			CWE:         "CWE-89",
			OWASP:       "A03:2021 - Injection",
			Languages:   []string{"python", "javascript", "typescript", "go", "php"},
			Patterns:    []string{"f\".*SELECT.*\\{", "f\".*INSERT.*\\{", "f\".*UPDATE.*\\{", "f\".*DELETE.*\\{"},
			Remediation: "Use parameterized queries with placeholders (? or %s)",
			Confidence:  "high",
			Category:    "injection",
		},
		// ── Command Injection ──────────────────────────────────
		{
			ID:          "SAST-003",
			Name:        "Command Injection",
			Description: "User input passed to system command execution",
			Severity:    "critical",
			CWE:         "CWE-78",
			OWASP:       "A03:2021 - Injection",
			Languages:   []string{"python", "javascript", "typescript", "go", "php", "ruby"},
			Patterns:    []string{"os.system(\\s*\\(", "subprocess.call(\\s*\\(", "subprocess.run(\\s*\\(", "exec(\\s*\\(", "execSync(\\s*\\(", "child_process"},
			Remediation: "Use parameterized subprocess calls with shell=False (Python) or avoid shell execution",
			Confidence:  "high",
			Category:    "injection",
		},
		// ── XSS (Cross-Site Scripting) ─────────────────────────
		{
			ID:          "SAST-004",
			Name:        "Reflected XSS",
			Description: "User input rendered in HTML without escaping",
			Severity:    "high",
			CWE:         "CWE-79",
			OWASP:       "A03:2021 - Injection",
			Languages:   []string{"javascript", "typescript", "python", "php", "ruby"},
			Patterns:    []string{"innerHTML\\s*[=:]", "document.write(\\s*\\(", "dangerouslySetInnerHTML", "html(\\s*\\(", "<%= .*%>", "\\.html(\\s*\\("},
			Remediation: "Use safe rendering methods (textContent, React JSX auto-escaping) or HTML sanitization",
			Confidence:  "high",
			Category:    "injection",
		},
		// ── Path Traversal ─────────────────────────────────────
		{
			ID:          "SAST-005",
			Name:        "Path Traversal",
			Description: "User input used to construct file paths without validation",
			Severity:    "high",
			CWE:         "CWE-22",
			OWASP:       "A01:2021 - Broken Access Control",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"open(\\s*\\(.*\\+", "readFile(\\s*\\(.*\\+", "readFileSync(\\s*\\(.*\\+", "os.path.join(\\s*\\(.*request", "path.join(\\s*\\(.*req"},
			Remediation: "Validate and sanitize file paths, use allowlist of permitted directories",
			Confidence:  "medium",
			Category:    "access-control",
		},
		// ── Hardcoded Secrets ──────────────────────────────────
		{
			ID:          "SAST-006",
			Name:        "Hardcoded API Key",
			Description: "API key or token hardcoded in source code",
			Severity:    "high",
			CWE:         "CWE-798",
			OWASP:       "A07:2021 - Identification and Authentication Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby", "yaml", "json"},
			Patterns:    []string{"api[_-]?key\\s*[=:]\\s*['\"][a-zA-Z0-9]{16,}", "apikey\\s*[=:]\\s*['\"][a-zA-Z0-9]{16,}", "api-key\\s*[=:]\\s*['\"][a-zA-Z0-9]{16,}"},
			Remediation: "Store secrets in environment variables, secret managers (Vault, AWS Secrets Manager, etc.)",
			Confidence:  "medium",
			Category:    "secrets",
		},
		{
			ID:          "SAST-007",
			Name:        "Hardcoded Password",
			Description: "Password hardcoded in source code",
			Severity:    "high",
			CWE:         "CWE-798",
			OWASP:       "A07:2021 - Identification and Authentication Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"password\\s*[=:]\\s*['\"][^'\"]{4,}", "passwd\\s*[=:]\\s*['\"][^'\"]{4,}", "pwd\\s*[=:]\\s*['\"][^'\"]{4,}"},
			Remediation: "Use environment variables or secret management solutions for credentials",
			Confidence:  "medium",
			Category:    "secrets",
		},
		{
			ID:          "SAST-008",
			Name:        "Hardcoded JWT/Auth Token",
			Description: "JWT or bearer token hardcoded in source code",
			Severity:    "high",
			CWE:         "CWE-798",
			OWASP:       "A07:2021 - Identification and Authentication Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"bearer\\s+[a-zA-Z0-9\\-_\\.]{20,}", "eyJ[a-zA-Z0-9\\-_]*\\.eyJ[a-zA-Z0-9\\-_]*", "token\\s*[=:]\\s*['\"][a-zA-Z0-9]{20,}"},
			Remediation: "Use environment variables or secure token storage mechanisms",
			Confidence:  "medium",
			Category:    "secrets",
		},
		// ── Insecure Crypto ────────────────────────────────────
		{
			ID:          "SAST-009",
			Name:        "Insecure Hash Algorithm (MD5)",
			Description: "Use of MD5 for cryptographic hashing",
			Severity:    "medium",
			CWE:         "CWE-327",
			OWASP:       "A02:2021 - Cryptographic Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"md5", "MD5"},
			Remediation: "Use SHA-256 or better for hashing; use bcrypt/Argon2 for password hashing",
			Confidence:  "medium",
			Category:    "crypto",
		},
		{
			ID:          "SAST-010",
			Name:        "Insecure Hash Algorithm (SHA1)",
			Description: "Use of SHA1 for cryptographic hashing",
			Severity:    "medium",
			CWE:         "CWE-327",
			OWASP:       "A02:2021 - Cryptographic Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"sha1", "SHA1", "sha-1", "SHA-1"},
			Remediation: "Use SHA-256 or better for hashing; use bcrypt/Argon2 for password hashing",
			Confidence:  "medium",
			Category:    "crypto",
		},
		{
			ID:          "SAST-011",
			Name:        "Weak Random Number Generator",
			Description: "Use of predictable random number generator",
			Severity:    "medium",
			CWE:         "CWE-338",
			OWASP:       "A02:2021 - Cryptographic Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"Math.random(\\s*\\(", "random.random(\\s*\\(", "rand(\\s*\\(", "rand.Int(\\s*\\("},
			Remediation: "Use cryptographically secure random generators (crypto/rand in Go, secrets in Python, crypto in Node.js)",
			Confidence:  "medium",
			Category:    "crypto",
		},
		// ── Insecure Deserialization ───────────────────────────
		{
			ID:          "SAST-012",
			Name:        "Insecure Deserialization",
			Description: "User-controlled data passed to deserialization function",
			Severity:    "critical",
			CWE:         "CWE-502",
			OWASP:       "A08:2021 - Software and Data Integrity Failures",
			Languages:   []string{"python", "javascript", "typescript", "java", "php", "ruby"},
			Patterns:    []string{"pickle.loads(\\s*\\(", "yaml.load(\\s*\\(", "unserialize(\\s*\\(", "ObjectInputStream", "JSON.parse(\\s*\\(.*req", "eval(\\s*\\("},
			Remediation: "Use safe deserialization methods (yaml.safe_load, json.loads with schema validation)",
			Confidence:  "high",
			Category:    "deserialization",
		},
		// ── SSRF ───────────────────────────────────────────────
		{
			ID:          "SAST-013",
			Name:        "Server-Side Request Forgery (SSRF)",
			Description: "User input used to construct outbound HTTP requests",
			Severity:    "high",
			CWE:         "CWE-918",
			OWASP:       "A10:2021 - Server-Side Request Forgery (SSRF)",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"requests.get(\\s*\\(.*\\+", "requests.post(\\s*\\(.*\\+", "axios.get(\\s*\\(.*\\+", "http.Get(\\s*\\(.*\\+", "fetch(\\s*\\(.*\\+"},
			Remediation: "Validate and sanitize URLs, use allowlist for permitted domains, disable URL redirects",
			Confidence:  "medium",
			Category:    "ssrf",
		},
		// ── Open Redirect ──────────────────────────────────────
		{
			ID:          "SAST-014",
			Name:        "Open Redirect",
			Description: "User input used in redirect without validation",
			Severity:    "medium",
			CWE:         "CWE-601",
			OWASP:       "A01:2021 - Broken Access Control",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"redirect(\\s*\\(.*request", "res.redirect(\\s*\\(.*\\+", "http.Redirect(\\s*\\(.*\\+", "Location.*\\+", "window.location"},
			Remediation: "Validate redirect URLs against allowlist, use relative redirects only",
			Confidence:  "medium",
			Category:    "access-control",
		},
		// ── Insecure TLS/SSL ───────────────────────────────────
		{
			ID:          "SAST-015",
			Name:        "Insecure TLS Configuration",
			Description: "Disabled or weak TLS certificate verification",
			Severity:    "high",
			CWE:         "CWE-295",
			OWASP:       "A02:2021 - Cryptographic Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "ruby"},
			Patterns:    []string{"verify=False", "verify_ssl=False", "rejectUnauthorized.*false", "InsecureSkipVerify.*true", "ssl_verify.*false"},
			Remediation: "Always enable TLS certificate verification; use custom CA bundle if needed",
			Confidence:  "high",
			Category:    "crypto",
		},
		// ── Debug/Development Code ─────────────────────────────
		{
			ID:          "SAST-016",
			Name:        "Debug Mode Enabled",
			Description: "Application running in debug mode in production",
			Severity:    "medium",
			CWE:         "CWE-489",
			OWASP:       "A05:2021 - Security Misconfiguration",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php"},
			Patterns:    []string{"debug=True", "DEBUG=True", "debug.*true", "app.debug(\\s*\\(\\s*true"},
			Remediation: "Disable debug mode in production; use environment variables to control debug settings",
			Confidence:  "high",
			Category:    "misconfiguration",
		},
		{
			ID:          "SAST-017",
			Name:        "Hardcoded Debug Credentials",
			Description: "Default or hardcoded credentials for development/debug",
			Severity:    "high",
			CWE:         "CWE-798",
			OWASP:       "A07:2021 - Identification and Authentication Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"admin.*admin", "root.*root", "password.*password", "123456", "password123", "default.*password"},
			Remediation: "Remove default credentials; enforce strong password policies; use OAuth/SSO",
			Confidence:  "low",
			Category:    "secrets",
		},
		// ── Logging Sensitive Data ─────────────────────────────
		{
			ID:          "SAST-018",
			Name:        "Sensitive Data in Logs",
			Description: "Sensitive information (passwords, tokens) written to logs",
			Severity:    "medium",
			CWE:         "CWE-532",
			OWASP:       "A09:2021 - Security Logging and Monitoring Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby"},
			Patterns:    []string{"log.*password", "log.*token", "log.*secret", "log.*api[_-]?key", "console.log(.*password", "fmt.Println(.*password", "logger.*password"},
			Remediation: "Sanitize logs before output; never log passwords, tokens, or PII",
			Confidence:  "medium",
			Category:    "logging",
		},
		// ── Race Conditions ────────────────────────────────────
		{
			ID:          "SAST-019",
			Name:        "Race Condition (TOCTOU)",
			Description: "Time-of-check to time-of-use vulnerability in file operations",
			Severity:    "medium",
			CWE:         "CWE-367",
			OWASP:       "A01:2021 - Broken Access Control",
			Languages:   []string{"c", "cpp", "go", "java", "python"},
			Patterns:    []string{"os.path.exists(\\s*\\(.*open(\\s*\\(", "access(\\s*\\(.*open(\\s*\\("},
			Remediation: "Use atomic file operations or file locking mechanisms",
			Confidence:  "low",
			Category:    "concurrency",
		},
		// ── HTTP Without TLS ───────────────────────────────────
		{
			ID:          "SAST-020",
			Name:        "HTTP Without TLS",
			Description: "Hardcoded HTTP URL instead of HTTPS",
			Severity:    "low",
			CWE:         "CWE-319",
			OWASP:       "A02:2021 - Cryptographic Failures",
			Languages:   []string{"python", "javascript", "typescript", "go", "java", "php", "ruby", "yaml", "json"},
			Patterns:    []string{"http://[^s]", "http://localhost", "http://127.0.0.1"},
			Remediation: "Use HTTPS for all external communications; use TLS everywhere",
			Confidence:  "low",
			Category:    "crypto",
		},
	}
}

// FilterRulesByLanguage returns rules applicable to the given languages
func FilterRulesByLanguage(rules []models.Rule, languages []string) []models.Rule {
	if len(languages) == 0 {
		return rules
	}
	langSet := make(map[string]bool)
	for _, l := range languages {
		langSet[l] = true
	}
	var filtered []models.Rule
	for _, r := range rules {
		for _, lang := range r.Languages {
			if langSet[lang] {
				filtered = append(filtered, r)
				break
			}
		}
	}
	return filtered
}

// FilterRulesBySeverity returns rules with severity >= given threshold
func FilterRulesBySeverity(rules []models.Rule, minSeverity string) []models.Rule {
	severityOrder := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	min := severityOrder[minSeverity]
	if min == 0 {
		min = 1
	}
	var filtered []models.Rule
	for _, r := range rules {
		if severityOrder[r.Severity] >= min {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
