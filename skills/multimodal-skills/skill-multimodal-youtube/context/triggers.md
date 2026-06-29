# Triggers

## Primary triggers (activate immediately)

- "youtube" + any verb: "search youtube", "youtube video", "youtube channel"
- "find video", "find me videos", "best video on", "top videos about"
- "watch this video", "summarise this video", "what does this video say"
- "transcript of", "get transcript", "video transcript"
- "clip video", "cut clip from video", "extract clip"
- "highlight reel", "combine clips", "merge clips"
- "download video", "save video", "save audio"
- "youtube playlist", "playlist videos"
- YouTube URL pasted: `youtube.com/watch?v=...`, `youtu.be/...`

## Secondary triggers (activate with context)

- "channel videos", "what did this YouTuber upload"
- "video metadata", "video info", "how many views"
- "subscriber count", "channel info"
- "best tutorials", "top tutorials" (when video context is implied)

## Anti-triggers (do NOT activate)

- "web search" without YouTube context → use `sin_web_search`
- "stream video", "play video in browser" → use `sin_browser_navigate`
- Non-YouTube video platforms (Vimeo, Twitch, etc.)
- "video file" referring to local files → use `sin_read` / `analyse__video`
