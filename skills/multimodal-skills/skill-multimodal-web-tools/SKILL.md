---
name: skill-multimodal-web-tools
description: "Use when agent needs to research anything on the web or YouTube. Master skill for ALL SIN-Code research tools: sin_web_search (free DuckDuckGo), websearch MCP (20+ sources), YouTube MCP (9 tools), sin_http_get (URL fetch). Triggers on 'research', 'search web', 'find online', 'youtube', 'look up', 'what's the latest on'."
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
  - cursor
  - gemini
metadata:
  author: OpenSIN-Code
  version: 3.26.0
  category: multimodal
  lifecycle: native
required_tools:
  - sin_web_search
  - sin_http_get
  - websearch__websearch_search
  - youtube__youtube_search
  - youtube__youtube_get_transcript
  - youtube__youtube_get_video_info
optional_tools:
  - websearch__websearch_alchemist
  - websearch__websearch_pulse
  - websearch__websearch_resolve
  - websearch__websearch_video_brief
  - websearch__websearch_video_prompt
  - websearch__websearch_watch
  - youtube__youtube_get_channel_videos
  - youtube__youtube_get_channel_info
  - youtube__youtube_get_playlist
  - youtube__youtube_download
  - youtube__youtube_clip
  - youtube__youtube_highlight_reel
---

# skill-multimodal-web-tools

Master skill for ALL SIN-Code research and web interaction tools. When an
agent needs to look something up online, search the web, find YouTube videos,
or fetch a URL — this skill provides the complete tool inventory and
decision tree for choosing the right tool.

## When to activate

Activate when the agent needs information from the internet:

- "search for...", "look up...", "research...", "find online..."
- "what's the latest on...", "what's new with..."
- "youtube", "find video", "watch video", "video tutorial"
- "fetch this URL", "read this page", "check this link"
- "deep research", "comprehensive analysis", "multi-source"
- Any time the agent would say "I don't have access to the internet" — IT DOES

Do **not** activate for:
- Reading local files (use `sin_read`)
- Searching code (use `sin_search` / `sin_scout`)
- Analyzing images (use `sin_analyse_image`)

## Tool inventory — 4 layers

### Layer 1: sin_web_search (BUILTIN — always available, no MCP needed)

**The default web search. Free. No API key. Zero config.**

| Property | Value |
|---|---|
| Tool name | `sin_web_search` |
| Permission | `allow` (read-only) |
| API key needed? | **NO** — DuckDuckGo is keyless |
| Providers | DuckDuckGo (always) + Tavily/SerpAPI/Brave (if env keys set) |
| Best for | Quick web searches, finding docs, checking facts |

**Parameters:**
- `query` (required) — search query
- `max` (default 10) — max results, 1-50
- `json` (default false) — structured JSON output

**When to use:** Default first choice for any web search. DuckDuckGo returns
suggestion-style results. If you need actual page content, follow up with
`sin_http_get` on the best result URL.

### Layer 2: sin_http_get (BUILTIN — fetch any URL)

**Fetch a URL and read its content. No API key.**

| Property | Value |
|---|---|
| Tool name | `sin_http_get` |
| Permission | `allow` (read-only) |
| API key needed? | **NO** |
| Limits | GET only, 256KB cap, 30s timeout |
| Best for | Reading docs pages, API responses, checking a specific URL |

**Parameters:**
- `url` (required) — http(s) URL

**When to use:** After `sin_web_search` to read the content of a result.
Or when you already have a URL and want to fetch its content.

### Layer 3: websearch MCP (7 tools — multi-source research engine)

**Professional multi-source research with 20+ sources. Requires MCP server.**

| Property | Value |
|---|---|
| MCP server | `sin-websearch` (binary at `~/.local/share/sin-code/skills/web_search_bundle/`) |
| Permission | `websearch__*` = `allow` (read-only) |
| API key needed? | SerpAPI keys configured in `~/.config/sin-websearch/sin-websearch.yaml` |
| Best for | Deep research, multi-source analysis, entity resolution |

**7 tools:**

| Tool | What it does |
|---|---|
| `websearch__websearch_search` | Search across 20+ sources (Google, Reddit, X, YouTube, GitHub, HN, SearxNG) |
| `websearch__websearch_alchemist` | Multi-agent research report (aggregates + synthesizes findings) |
| `websearch__websearch_pulse` | Topic pulse — trends, sentiment, volume over time |
| `websearch__websearch_resolve` | Entity resolution — extract people, repos, companies from a query |
| `websearch__websearch_video_brief` | Find + summarize YouTube videos on a topic |
| `websearch__websearch_video_prompt` | Generate a video creation prompt from research |
| `websearch__websearch_watch` | Monitor a topic for changes |

**When to use:** When you need comprehensive multi-source research beyond
what DuckDuckGo provides. When you need Reddit, X/Twitter, GitHub, HN results.
When you need a full research report (`alchemist`).

### Layer 4: YouTube MCP (9 tools — full YouTube interaction)

**Search, watch, clip, and build highlight reels. No YouTube Data API key.**

| Property | Value |
|---|---|
| MCP server | `youtube` (Node.js at `~/dev/youtube-for-ai-agents/dist/index.js`) |
| Permission | 6 read-only `allow` + 3 action `ask` |
| API key needed? | **NO** — uses youtubei.js InnerTube client |
| Best for | Finding videos, watching content, cutting clips, highlight reels |

**9 tools:**

| Tool | Permission | What it does |
|---|---|---|
| `youtube__youtube_search` | allow | Search videos/channels/playlists |
| `youtube__youtube_get_transcript` | allow | Get transcript with timestamps |
| `youtube__youtube_get_video_info` | allow | Video metadata (brief/standard/full) |
| `youtube__youtube_get_channel_videos` | allow | List channel videos (newest/popular/oldest) |
| `youtube__youtube_get_channel_info` | allow | Channel metadata |
| `youtube__youtube_get_playlist` | allow | Playlist metadata + video list |
| `youtube__youtube_download` | ask | Download video/audio to file |
| `youtube__youtube_clip` | ask | Cut clips by timestamp |
| `youtube__youtube_highlight_reel` | ask | Merge clips into highlight reel |

**When to use:** When the user asks about YouTube videos specifically.
When you need to "watch" a video (read its transcript). When you need to
cut clips or build highlight reels.

## Decision tree — which tool to use?

```
User wants to research something
│
├─ Quick web search?
│  └─ sin_web_search {query: "..."}
│     └─ Need page content? → sin_http_get {url: "<best result>"}
│
├─ Deep multi-source research?
│  └─ websearch__websearch_search {query: "..."}
│     └─ Need full report? → websearch__websearch_alchemist {query: "..."}
│     └─ Need entity extraction? → websearch__websearch_resolve {query: "..."}
│
├─ YouTube video?
│  ├─ Find videos → youtube__youtube_search {query: "...", sortBy: "rating"}
│  ├─ Watch video → youtube__youtube_get_transcript {videoId: "..."}
│  ├─ Video info → youtube__youtube_get_video_info {videoId: "...", detail: "brief"}
│  ├─ Channel videos → youtube__youtube_get_channel_videos {channel: "@handle", sort: "popular"}
│  ├─ Cut clip → youtube__youtube_clip {videoId: "...", clips: [{startTime, endTime, label}]}
│  └─ Highlight reel → youtube__youtube_highlight_reel {clips: ["path1", "path2"]}
│
├─ Fetch specific URL?
│  └─ sin_http_get {url: "https://..."}
│
└─ All of the above?
   └─ Start with sin_web_search, then websearch__websearch_search,
      then youtube__youtube_search if video content is needed
```

## Workflow: Comprehensive research

```
1. sin_web_search          → quick DuckDuckGo results
2. websearch__websearch_search → deep multi-source results
3. youtube__youtube_search    → video results
4. sin_http_get             → read top result pages
5. youtube__youtube_get_transcript → watch top video
6. Synthesize all findings into a report
```

## Permission policy

```
sin_web_search              →  allow  (builtin, read-only)
sin_http_get               →  allow  (builtin, read-only)
websearch__*               →  allow  (MCP, read-only)
youtube__youtube_search    →  allow  (read-only)
youtube__youtube_get_*     →  allow  (read-only, 5 tools)
youtube__youtube_download  →  ask    (writes files, M4)
youtube__youtube_clip      →  ask    (downloads + cuts, M4)
youtube__youtube_highlight_reel → ask (merges files, M4)
```

## API keys (all optional)

| Key | Env var | Config location | Provider |
|---|---|---|---|
| Tavily | `WEBSEARCH_TAVILY_KEY` | `~/.zshrc` | AI-optimized search (sin_web_search) |
| SerpAPI | `WEBSEARCH_SERPAPI_KEY` | `~/.zshrc` + `~/.config/sin-websearch/sin-websearch.yaml` | Aggregator (sin_web_search + websearch MCP) |
| Brave | `WEBSEARCH_BRAVE_KEY` | `~/.zshrc` | Search engine (sin_web_search) |
| YouTube cookies | `YOUTUBE_COOKIE_PATH` | `~/.config/sin-youtube/cookies.json` | Age-restricted content (youtube MCP) |

**All keys are optional.** DuckDuckGo works without any keys. YouTube works
without cookies. Keys are stored in Infisical and loaded via the sin-infisical skill.
