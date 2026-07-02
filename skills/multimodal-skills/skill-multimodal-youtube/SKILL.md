---
name: skill-multimodal-youtube
description: "Use when user says 'youtube', 'search youtube', 'find video', 'watch video', 'video transcript', 'clip video', 'highlight reel', 'youtube channel', 'youtube playlist'. Full YouTube interaction for AI agents: search, watch, transcript, clip, and reel — no YouTube Data API key needed."
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
lifecycle: external
sources: "JCodesMore/youtube-for-ai-agents"
required_tools:
  - youtube__youtube_search
  - youtube__youtube_get_transcript
  - youtube__youtube_get_video_info
  - youtube__youtube_get_channel_videos
  - youtube__youtube_get_channel_info
  - youtube__youtube_get_playlist
optional_tools:
  - youtube__youtube_download
  - youtube__youtube_clip
  - youtube__youtube_highlight_reel
---

# skill-multimodal-youtube

Full YouTube interaction for AI agents. Search, watch, transcript, clip,
and build highlight reels — all without leaving the chat. No YouTube Data
API key required. Uses `youtubei.js` InnerTube client under the hood.

## When to activate

Activate when the user wants to interact with YouTube content:

- "search YouTube for...", "find me videos on...", "best video about..."
- "watch this video and tell me...", "summarise this YouTube video"
- "get the transcript of...", "what does this video say about..."
- "find the best videos from this channel", "what's worth watching from..."
- "clip this video at 2:15", "cut a 30-second clip from..."
- "combine these clips into a highlight reel"
- "download this video", "save the audio of this video"
- "what's in this playlist", "list videos from playlist..."
- User pastes a YouTube URL (youtube.com/watch?v=..., youtu.be/...)

Do **not** activate for:

- General web search (use `sin_web_search` instead)
- Non-YouTube video platforms
- Playing videos in a browser (use `sin_browser_navigate` instead)

## The 9 YouTube tools

### Read-only tools (allow — no permission prompt)

| Tool | What it does | Key parameters |
|---|---|---|
| `youtube__youtube_search` | Search videos, channels, or playlists | `query`, `maxResults` (default 20, max 50), `type` (video/channel/playlist), `uploadDate` (today/week/month/year), `duration` (short/medium/long), `sortBy` (relevance/date/views/rating) |
| `youtube__youtube_get_transcript` | Get transcript with timestamp control | `videoId` or URL, `format` (text/segments/both), `startTime` (seconds), `endTime` (seconds), `maxSegments`, `language` (default en) |
| `youtube__youtube_get_video_info` | Get video metadata | `videoId` or URL, `detail` (brief/standard/full) — brief = title/channel/views/likes/duration; full = everything including chapters, tags, thumbnail |
| `youtube__youtube_get_channel_videos` | List videos from a channel | `channel` (@handle, URL, or ID), `limit` (default 30, max 500), `sort` (newest/popular/oldest) |
| `youtube__youtube_get_channel_info` | Get channel metadata | `channel` (@handle, URL, or ID) — name, subscriber count, description, country |
| `youtube__youtube_get_playlist` | Get playlist metadata + videos | `playlistId` or URL, `limit` (default 30, max 200) |

### Action tools (ask — permission-gated per M4)

| Tool | What it does | Key parameters |
|---|---|---|
| `youtube__youtube_download` | Download video or audio to local file | `videoId` or URL, `quality` (720p/1080p/best), `type` (video+audio/audio/video), `format`, `force` (bypass 30min confirmation) |
| `youtube__youtube_clip` | Extract clips by timestamp | `videoId` or URL, `clips` array: each with `startTime`, `endTime` (seconds, MM:SS, or HH:MM:SS), `label`. 2+ clips auto-generates a highlight reel. `accurate` = frame-perfect (slower, default off) |
| `youtube__youtube_highlight_reel` | Combine clip files into one reel | `clips` array of file paths from previous `youtube_clip` results, `output` filename |

## Mandatory workflow

```
SEARCH  ->  ASSESS  ->  WATCH  ->  REPORT
```

1. **SEARCH** — call `youtube__youtube_search` with the user's query.
   Use `sortBy: "rating"` for "best" videos, `sortBy: "views"` for
   popular, `sortBy: "relevance"` (default) for general searches.
   Filter by `uploadDate: "month"` for recent content.

2. **ASSESS** — for each top result, call `youtube__youtube_get_video_info`
   with `detail: "brief"` to get title, channel, views, likes, duration.
   Present a ranked table to the user.

3. **WATCH** — call `youtube__youtube_get_transcript` to read the video
   content. Use `format: "text"` for a compact summary, or
   `format: "segments"` when the user needs specific timestamps.
   Use `startTime`/`endTime` to focus on a section.

4. **REPORT** — summarise the key points, actionable takeaways, or
   relevant quotes. If the user wants clips, use `youtube__youtube_clip`
   with tight 5-10 second segments capturing one key moment each.

## Cookie login (optional — for age-restricted/personalized content)

By default, search runs anonymously — no login required. For personalized
recommendations, age-restricted videos, or member-only content:

### Automated extraction (recommended — no user interaction)

Agents can extract YouTube cookies autonomously from the local browser:

```bash
# Install browser_cookie3 (one-time)
pip3 install --break-system-packages browser_cookie3

# Extract cookies from Chrome/Safari/Firefox + store locally + Infisical
python3 scripts/auto-extract-cookies.py --store-infisical
```

The script:
1. Reads YouTube cookies directly from the browser's cookie database
   (Chrome first, then Safari, then Firefox — uses `browser_cookie3`)
2. Saves to `~/.config/sin-youtube/cookies.json` (chmod 600)
3. With `--store-infisical`: stores as `YOUTUBE_COOKIES_JSON` in Infisical
   (so other Macs can download via `cookie-setup.sh`)

On other Macs, download from Infisical:
```bash
bash scripts/cookie-setup.sh
```

### Manual extraction (fallback)

If automated extraction fails (e.g. no browser on the machine):

1. Open Chrome/Safari → youtube.com → make sure you're logged in
2. DevTools → Application → Cookies → youtube.com
3. Copy these cookies as JSON: `SID`, `HSID`, `SSID`, `APISID`, `SAPISID`,
   `__Secure-1PSID`, `__Secure-3PSID`, `LOGIN_INFO`
4. Save to `~/.config/sin-youtube/cookies.json` (chmod 600)
5. Store in Infisical: `bash scripts/cookie-store.sh ~/.config/sin-youtube/cookies.json`

### Configuration

- Cookie file: `~/.config/sin-youtube/cookies.json` (chmod 600)
- Env var: `YOUTUBE_COOKIE_PATH` to override the default path
- Infisical secret: `YOUTUBE_COOKIES_JSON`

Never log, echo, or commit cookie values. Treat them as secrets (M4).

## Permission policy

```
youtube__youtube_search              ->  allow  (read-only)
youtube__youtube_get_transcript      ->  allow  (read-only)
youtube__youtube_get_video_info      ->  allow  (read-only)
youtube__youtube_get_channel_videos  ->  allow  (read-only)
youtube__youtube_get_channel_info    ->  allow  (read-only)
youtube__youtube_get_playlist        ->  allow  (read-only)
youtube__youtube_download            ->  ask    (writes files, M4)
youtube__youtube_clip                ->  ask    (downloads + cuts, M4)
youtube__youtube_highlight_reel      ->  ask    (merges files, M4)
```

## Skill coupling

This skill cooperates with:

- `sin_web_search` — for non-YouTube web searches
- `sin_browser_navigate` — for playing videos in a browser
- `sin_bash` — for managing downloaded files
- `sin_edit` / `sin_write` — for persisting transcripts or summaries

## Examples

### "Find me the best video on Go testing"

```
1. youtube__youtube_search {query: "Go testing tutorial", sortBy: "rating", maxResults: 5}
2. youtube__youtube_get_video_info {videoId: "<top result>", detail: "brief"}
3. youtube__youtube_get_transcript {videoId: "<top result>", format: "text"}
4. Summarise key testing patterns from the transcript
```

### "Cut a 30-second clip from 2:15 of this video"

```
1. youtube__youtube_clip {videoId: "abc123", clips: [{startTime: "2:15", endTime: "2:45", label: "key-point"}]}
```

### "Combine these three clips into a highlight reel"

```
1. youtube__youtube_highlight_reel {clips: ["/path/clip1.mp4", "/path/clip2.mp4", "/path/clip3.mp4"]}
```

### "What are this channel's best videos?"

```
1. youtube__youtube_get_channel_info {channel: "@Fireship"}
2. youtube__youtube_get_channel_videos {channel: "@Fireship", sort: "popular", limit: 10}
3. youtube__youtube_get_transcript {videoId: "<top video>", format: "text"} for top 3
4. Summarise each
```
