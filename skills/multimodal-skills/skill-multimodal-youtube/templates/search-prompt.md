# YouTube Search Prompt

Use this prompt template when searching YouTube for the user:

```
I'll search YouTube for "{query}" and find the most relevant videos.

Steps:
1. Call youtube__youtube_search with query="{query}", maxResults={max}, sortBy="{sort}"
2. For each result, extract: title, channel, views, duration, URL
3. Present a ranked table
4. If the user wants to watch any, call youtube__youtube_get_transcript
```

## Clip prompt

```
I'll clip the video at the specified timestamps.

Steps:
1. Call youtube__youtube_clip with the video ID and clip specifications
2. Present the resulting file paths
3. If 2+ clips were cut, an auto-generated highlight reel is also produced
```
