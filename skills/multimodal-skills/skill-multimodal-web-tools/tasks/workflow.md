# Research Workflow

## Quick search (default — 90% of cases)

```
1. sin_web_search {query: "...", max: 10}
2. Pick best result → sin_http_get {url: "<result URL>"}
3. Summarize for user
```

## Deep research (multi-source)

```
1. sin_web_search {query: "..."}           → DuckDuckGo quick results
2. websearch__websearch_search {query: "..."} → 20+ sources
3. youtube__youtube_search {query: "..."}    → video results
4. sin_http_get {url: "<top result>"}        → read page content
5. youtube__youtube_get_transcript {videoId: "<top video>"} → watch video
6. Synthesize all findings into report
```

## YouTube research

```
1. youtube__youtube_search {query: "...", sortBy: "rating", maxResults: 5}
2. youtube__youtube_get_video_info {videoId: "<top>", detail: "brief"}
3. youtube__youtube_get_transcript {videoId: "<top>", format: "text"}
4. Summarize key points
```

## YouTube clip + reel

```
1. youtube__youtube_get_transcript {videoId: "...", format: "segments"}
2. Identify key timestamps from transcript
3. youtube__youtube_clip {videoId: "...", clips: [
     {startTime: "2:15", endTime: "2:45", label: "insight-1"},
     {startTime: "5:30", endTime: "5:50", label: "insight-2"},
     {startTime: "8:10", endTime: "8:30", label: "insight-3"}
   ]}
4. If clips span multiple videos:
   youtube__youtube_highlight_reel {clips: ["path1.mp4", "path2.mp4", "path3.mp4"]}
```

## Channel browsing

```
1. youtube__youtube_get_channel_info {channel: "@handle"}
2. youtube__youtube_get_channel_videos {channel: "@handle", sort: "popular", limit: 10}
3. youtube__youtube_get_transcript {videoId: "<top video>", format: "text"}
4. Summarize each video
```

## Full research report (alchemist)

```
1. websearch__websearch_alchemist {query: "...", depth: "deep"}
   → Multi-agent research report aggregating 20+ sources
2. youtube__youtube_search {query: "...", maxResults: 3}
   → Video perspective
3. youtube__youtube_get_transcript {videoId: "<top>", format: "text"}
4. Combine alchemist report + video transcript into final summary
```
