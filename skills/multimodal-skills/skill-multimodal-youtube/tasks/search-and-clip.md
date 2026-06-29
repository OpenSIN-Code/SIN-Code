# Search and Clip Task

## Search for videos

1. Call `youtube__youtube_search` with the query
2. Present results in a table with title, channel, views, duration, URL
3. If user wants more detail, call `youtube__youtube_get_video_info`

## Clip a video

1. Get the video ID from a URL or search result
2. Identify the clip timestamps (from transcript or user-specified)
3. Call `youtube__youtube_clip`:

```json
{
  "videoId": "abc123",
  "clips": [
    {"startTime": "2:15", "endTime": "2:45", "label": "key-insight"},
    {"startTime": "5:30", "endTime": "5:45", "label": "demo"}
  ]
}
```

4. The tool returns file paths for each clip + an auto-generated reel (if 2+ clips)

## Build a cross-video highlight reel

1. Clip from multiple videos (call `youtube__youtube_clip` for each)
2. Collect all clip file paths
3. Call `youtube__youtube_highlight_reel`:

```json
{
  "clips": ["/path/clip1.mp4", "/path/clip2.mp4", "/path/clip3.mp4"],
  "output": "best-moments.mp4"
}
```
