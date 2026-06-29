# YouTube Interaction Workflow

## Search → Watch → Report

```
SEARCH  ->  ASSESS  ->  WATCH  ->  REPORT
```

### Step 1: SEARCH

Call `youtube__youtube_search` with the user's query.

- Use `sortBy: "rating"` when user says "best" or "top"
- Use `sortBy: "views"` for popularity
- Use `sortBy: "relevance"` for general searches (default)
- Use `uploadDate: "month"` for recent content
- Use `uploadDate: "week"` for this week's uploads
- Use `duration: "short"` for quick content (<4min)
- Use `duration: "long"` for deep dives (>20min)
- Default `maxResults: 20` covers most cases; reduce to 5-10 for focused answers

### Step 2: ASSESS

For each top result (top 3-5), call `youtube__youtube_get_video_info`
with `detail: "brief"` to get title, channel, views, likes, duration.

Present a ranked table:

```
| # | Title | Channel | Views | Duration |
|---|-------|---------|-------|----------|
| 1 | ...   | ...     | ...   | ...      |
```

### Step 3: WATCH

Call `youtube__youtube_get_transcript` to read the video content.

- `format: "text"` — compact, best for summarisation
- `format: "segments"` — timestamped, best for finding specific moments
- `format: "both"` — full detail (default)
- Use `startTime` / `endTime` to focus on a section
- Use `maxSegments: 50` to preview a long video before reading all

### Step 4: REPORT

Summarise key points, actionable takeaways, or relevant quotes.

If the user wants clips:
1. Identify the key moments (timestamps) from the transcript
2. Call `youtube__youtube_clip` with tight 5-10 second segments
3. Each clip needs `startTime`, `endTime`, and an optional `label`
4. 2+ clips auto-generates a highlight reel

If the user wants a cross-video highlight reel:
1. Clip from multiple videos using `youtube__youtube_clip`
2. Collect all clip file paths from the results
3. Call `youtube__youtube_highlight_reel` with all paths in desired order

## Channel browsing workflow

```
CHANNEL_INFO  ->  CHANNEL_VIDEOS  ->  RANK  ->  WATCH_TOP
```

1. `youtube__youtube_get_channel_info` — get channel metadata
2. `youtube__youtube_get_channel_videos` with `sort: "popular"` — best videos
3. Present ranked list
4. `youtube__youtube_get_transcript` for top 3 — summarise each

## Playlist workflow

```
PLAYLIST  ->  LIST_VIDEOS  ->  WATCH_SELECTED
```

1. `youtube__youtube_get_playlist` — get playlist metadata + video list
2. Present the video list
3. `youtube__youtube_get_transcript` for user-selected videos
