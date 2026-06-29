# Complete Tool Reference

## Layer 1: sin_web_search (BUILTIN)

```
Tool: sin_web_search
Permission: allow
API key: NO (DuckDuckGo free, Tavily/SerpAPI/Brave optional)
```

| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Search query |
| max | int | 10 | Max results (1-50) |
| json | bool | false | Structured JSON output |

**Providers activated by env keys:**
- `WEBSEARCH_TAVILY_KEY` → Tavily (AI-optimized)
- `WEBSEARCH_SERPAPI_KEY` → SerpAPI (aggregator)
- `WEBSEARCH_BRAVE_KEY` → Brave Search
- DuckDuckGo → always on, keyless

**Output:** Ranked results with title, URL, snippet, source, score.

## Layer 2: sin_http_get (BUILTIN)

```
Tool: sin_http_get
Permission: allow
API key: NO
```

| Parameter | Type | Default | Description |
|---|---|---|---|
| url | string | (required) | http(s) URL |

**Limits:** GET only, 256KB response cap, 30s timeout.

## Layer 3: websearch MCP (7 tools)

```
MCP server: sin-websearch
Permission: websearch__* = allow
API key: SerpAPI keys in ~/.config/sin-websearch/sin-websearch.yaml
```

### websearch__websearch_search
| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Search query |
| max_results | int | 10 | Max results per source |

Sources: Google, Reddit, X/Twitter, YouTube, GitHub, HackerNews, SearxNG, Perplexity.

### websearch__websearch_alchemist
| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Research topic |
| depth | string | "standard" | quick/standard/deep |

Multi-agent research report — aggregates and synthesizes findings across sources.

### websearch__websearch_pulse
| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Topic to monitor |
| timeframe | string | "7d" | Time window |

Trends, sentiment, volume over time.

### websearch__websearch_resolve
| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Entity to resolve |

Extracts people, repos, companies, handles from a query.

### websearch__websearch_video_brief
| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Video topic |
| max_videos | int | 5 | Max videos to summarize |

Finds YouTube videos and summarizes their transcripts.

### websearch__websearch_video_prompt
| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Topic |

Generates a video creation prompt from research findings.

### websearch__websearch_watch
| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Topic to watch |
| interval | string | "1h" | Check interval |

Monitors a topic for changes.

## Layer 4: YouTube MCP (9 tools)

```
MCP server: youtube
Permission: 6 allow + 3 ask
API key: NO (youtubei.js InnerTube client)
Cookies: optional (~/.config/sin-youtube/cookies.json)
```

### youtube__youtube_search (allow)
| Parameter | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Search query |
| maxResults | int | 20 | Max results (1-50) |
| type | string | "video" | video/channel/playlist |
| uploadDate | string | "all" | today/week/month/year |
| duration | string | "all" | short/medium/long |
| sortBy | string | "relevance" | relevance/date/views/rating |

### youtube__youtube_get_transcript (allow)
| Parameter | Type | Default | Description |
|---|---|---|---|
| videoId | string | (required) | Video ID or URL |
| format | string | "both" | text/segments/both |
| startTime | int | 0 | Start seconds |
| endTime | int | 0 | End seconds (0=end) |
| maxSegments | int | 5000 | Segment cap |
| language | string | "en" | Language code |

### youtube__youtube_get_video_info (allow)
| Parameter | Type | Default | Description |
|---|---|---|---|
| videoId | string | (required) | Video ID or URL |
| detail | string | "standard" | brief/standard/full |

### youtube__youtube_get_channel_videos (allow)
| Parameter | Type | Default | Description |
|---|---|---|---|
| channel | string | (required) | @handle, URL, or ID |
| limit | int | 30 | Max videos (1-500) |
| sort | string | "newest" | newest/popular/oldest |

### youtube__youtube_get_channel_info (allow)
| Parameter | Type | Default | Description |
|---|---|---|---|
| channel | string | (required) | @handle, URL, or ID |

### youtube__youtube_get_playlist (allow)
| Parameter | Type | Default | Description |
|---|---|---|---|
| playlistId | string | (required) | Playlist ID or URL |
| limit | int | 30 | Max videos (1-200) |

### youtube__youtube_download (ask — M4)
| Parameter | Type | Default | Description |
|---|---|---|---|
| videoId | string | (required) | Video ID or URL |
| quality | string | "720p" | 144p-2160p/best |
| type | string | "video+audio" | video+audio/audio/video |
| format | string | "mp4" | Output format |
| force | bool | false | Bypass 30min confirmation |

### youtube__youtube_clip (ask — M4)
| Parameter | Type | Default | Description |
|---|---|---|---|
| videoId | string | (required) | Video ID or URL |
| clips | array | (required) | [{startTime, endTime, label}] |
| accurate | bool | false | Frame-perfect (slower) |

Time formats: seconds (135), MM:SS (2:15), HH:MM:SS (1:02:15).
2+ clips auto-generates a highlight reel.

### youtube__youtube_highlight_reel (ask — M4)
| Parameter | Type | Default | Description |
|---|---|---|---|
| clips | array | (required) | File paths from previous youtube_clip |
| output | string | "highlight-reel.mp4" | Output filename |
