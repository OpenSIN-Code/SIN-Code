# Standards

## No API key required

This skill uses `youtubei.js` (YouTube InnerTube client) — not the YouTube
Data API v3. No API key, no project setup, no quota limits. Works out of
the box with zero configuration.

## Optional cookie login

For personalized search, recommendations, or age-restricted content:

1. Set `YOUTUBE_COOKIE_PATH` env var to `~/.config/sin-youtube/cookies.json`
2. Ensure the cookie file contains the required cookies:
   `SID`, `HSID`, `SSID`, `APISID`, `SAPISID`, `__Secure-1PSID`,
   `__Secure-3PSID`, `LOGIN_INFO`
3. File must be chmod 600

## MCP server setup

The YouTube MCP server is registered in sin-code's `mcpclient/config.go`:
- Command: `node /Users/jeremy/dev/youtube-for-ai-agents/dist/index.js`
- Override path: `SIN_YOUTUBE_MCP_PATH` env var

In opencode, it's in `~/.config/opencode/opencode.json` under `mcp.youtube`.

## Video ID extraction

All tools accept either a video ID (`dQw4w9WgXcQ`) or a full URL:
- `https://www.youtube.com/watch?v=dQw4w9WgXcQ`
- `https://youtu.be/dQw4w9WgXcQ`
- `https://www.youtube.com/embed/dQw4w9WgXcQ`
