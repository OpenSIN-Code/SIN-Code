# Triggers

## Primary triggers (activate immediately)

- "search for", "look up", "research", "find online", "google"
- "what's the latest on", "what's new with", "current state of"
- "youtube", "find video", "video tutorial", "watch video"
- "fetch this URL", "read this page", "check this link"
- "deep research", "comprehensive analysis", "multi-source"
- "clip video", "highlight reel", "transcript"
- "reddit", "hacker news", "twitter search", "X search"
- Any URL pasted (http/https)

## Secondary triggers

- "is there a video about", "someone said on youtube"
- "trending", "popular videos about"
- "what does the internet say about"
- "summarise this webpage", "read this article"

## Anti-triggers (do NOT activate)

- Reading local files → use `sin_read`
- Searching code → use `sin_search` / `sin_scout`
- Analyzing images → use `sin_analyse_image`
- Running shell commands → use `sin_bash`
