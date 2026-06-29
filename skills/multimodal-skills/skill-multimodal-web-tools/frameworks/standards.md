# Standards

## No API key required for basic usage

All tools work with zero configuration:
- `sin_web_search` — DuckDuckGo is keyless, always available
- `sin_http_get` — plain HTTP GET, no key
- `websearch__*` — SerpAPI keys in config file, but SearxNG fallback works keyless
- `youtube__*` — youtubei.js InnerTube client, no YouTube Data API key

## Optional keys for enhanced results

| Key | Env var | Provider | Impact |
|---|---|---|---|
| Tavily | `WEBSEARCH_TAVILY_KEY` | AI-optimized search | Better snippets, content extraction |
| SerpAPI | `WEBSEARCH_SERPAPI_KEY` | Google aggregator | Full Google results |
| Brave | `WEBSEARCH_BRAVE_KEY` | Brave Search | Independent search index |
| YouTube cookies | `YOUTUBE_COOKIE_PATH` | Age-restricted content | Unlocks restricted videos |

Keys are stored in Infisical and loaded via sin-infisical skill.

## MCP server locations

| Server | Binary | Config |
|---|---|---|
| sin-websearch | `~/.local/share/sin-code/skills/web_search_bundle/sin-websearch` | `~/.config/sin-websearch/sin-websearch.yaml` |
| youtube | `~/dev/youtube-for-ai-agents/dist/index.js` | `YOUTUBE_COOKIE_PATH` env |

Both are registered in:
- `~/.config/opencode/opencode.json` (opencode CLI)
- `Infra-SIN-OpenCode-Stack/opencode.json` (repo config)
- `cmd/sin-code/internal/mcpclient/config.go` (sin-code internal)

## Rate limiting

- DuckDuckGo: no rate limit (suggestions endpoint)
- Tavily: plan-dependent (dev keys are limited)
- SerpAPI: 100 searches/month per key (4 keys rotated)
- YouTube: no official limit (InnerTube is unofficial)
- sin_http_get: 256KB cap, 30s timeout per request
