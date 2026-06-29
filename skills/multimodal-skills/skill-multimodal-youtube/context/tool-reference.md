# YouTube Tool Reference

## Architecture

The YouTube MCP server (`youtube-for-ai-agents`) runs as a Node.js stdio
subprocess. It uses `youtubei.js` (an unofficial YouTube InnerTube client)
to access YouTube without the YouTube Data API v3 — no API key, no quota
limits, no project setup.

## Tool inventory

### youtube__youtube_search

Search YouTube for videos, channels, or playlists.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| query | string | (required) | Search query |
| maxResults | int | 20 | Max results (1-50) |
| type | string | "video" | video / channel / playlist |
| uploadDate | string | "all" | today / week / month / year |
| duration | string | "all" | short (<4min) / medium (4-20min) / long (>20min) |
| sortBy | string | "relevance" | relevance / date / views / rating |

**Returns:** JSON array of results with `id`, `title`, `channel`, `views`,
`published`, `duration`, `thumbnail`, `snippets`.

### youtube__youtube_get_transcript

Get the transcript/subtitles of a YouTube video.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| videoId | string | (required) | Video ID or full URL |
| format | string | "both" | text (fullText only) / segments (timestamps only) / both |
| startTime | int | 0 | Start time in seconds |
| endTime | int | 0 | End time in seconds (0 = until end) |
| maxSegments | int | 5000 | Cap for long videos |
| language | string | "en" | Preferred language code |

**Returns:** Transcript text and/or timestamped segments.

### youtube__youtube_get_video_info

Get metadata for a single video.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| videoId | string | (required) | Video ID or full URL |
| detail | string | "standard" | brief / standard / full |

**brief:** title, channel, views, likes, duration only.
**standard:** most fields, truncated description, chapter count.
**full:** everything including full description, chapters, tags, thumbnail URL.

### youtube__youtube_get_channel_videos

List videos from a YouTube channel.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| channel | string | (required) | @handle, full URL, or channel ID |
| limit | int | 30 | Max videos (1-500) |
| sort | string | "newest" | newest / popular / oldest |

### youtube__youtube_get_channel_info

Get metadata for a YouTube channel.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| channel | string | (required) | @handle, full URL, or channel ID |

**Returns:** name, handle, description, subscriber count, video count, country, thumbnails.

### youtube__youtube_get_playlist

Get a YouTube playlist's metadata and its videos.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| playlistId | string | (required) | Playlist ID or full URL |
| limit | int | 30 | Max videos (1-200) |

### youtube__youtube_download

Download a YouTube video or audio track to a local file.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| videoId | string | (required) | Video ID or full URL |
| quality | string | "720p" | 144p/240p/360p/480p/720p/1080p/1440p/2160p/best |
| type | string | "video+audio" | video+audio / audio / video |
| format | string | "mp4" | Output format |
| force | bool | false | Bypass 30-minute confirmation |

**Permission:** ask (M4 — writes files to disk)

### youtube__youtube_clip

Extract one or more clips from a YouTube video.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| videoId | string | (required) | Video ID or full URL |
| clips | array | (required) | Array of {startTime, endTime, label} |
| accurate | bool | false | Frame-perfect cuts (slower — re-encodes) |

**Time formats:** seconds (`135`), MM:SS (`2:15`), HH:MM:SS (`1:02:15`).

When 2+ clips are provided, automatically produces a per-video highlight
reel alongside individual clips.

**Permission:** ask (M4 — downloads + cuts video)

### youtube__youtube_highlight_reel

Combine existing clip files into a single highlight reel.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| clips | array | (required) | Array of file paths from previous youtube_clip results |
| output | string | "highlight-reel.mp4" | Output filename |

**Permission:** ask (M4 — merges files)
