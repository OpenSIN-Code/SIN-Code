# Security Policy

## Supported Versions

| Version | Supported |
|:---|:---|
| Latest (main) | ✅ |
| Previous release | ✅ |
| < 2026.04.01 | ❌ |

## Reporting a Vulnerability

If you discover a security vulnerability in Simone MCP, please:

1. **DO NOT** open a public issue
2. Email us at: security@opensin.ai
3. Include the following information:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact assessment
   - Suggested fix (if any)

We will respond within **48 hours** and work with you to resolve the issue responsibly.

## Security Best Practices

### OAuth 2.1
- Never commit OAuth tokens or JWKS keys to the repository
- Use environment variables for all sensitive configuration
- Validate `SIMONE_ALLOWED_ORIGINS` for production deployments
- Set `SIMONE_OAUTH_AUDIENCE` and `SIMONE_OAUTH_ISSUER` correctly

### API Security
- Keep `SIMONE_AUTH_REQUIRED=true` in production
- Rotate OAuth credentials regularly
- Use HTTPS for all remote deployments
- Restrict access to Qdrant, Neo4j, and Supabase endpoints

### Code Security
- Review all dependency updates for security advisories
- Run `pytest tests/ -v` before deploying changes
- Never expose internal endpoints (Qdrant, Neo4j) publicly
- Use Docker container isolation for production deployments

### Dependencies
- Monitor Python dependencies for known vulnerabilities
- Keep FastAPI, Python, and all libraries up to date
- Review Dockerfile base image security patches regularly

## Acknowledgments

We appreciate responsible disclosure from the security community and will credit researchers who report valid security issues (with their permission).
