# Research Output Template

## Quick search results

```
## Web Search: "{query}"

**Source:** {DuckDuckGo/SerpAPI/Tavily/Brave}
**Results:** {N} from {M} providers ({X}ms)

| # | Title | Source | URL |
|---|-------|--------|-----|
| 1 | {title} | {source} | {url} |
| 2 | {title} | {source} | {url} |

## Top result summary
{fetched page content summary via sin_http_get}
```

## YouTube results

```
## YouTube Search: "{query}"

| # | Title | Channel | Views | Duration | URL |
|---|-------|---------|-------|----------|-----|
| 1 | {title} | {channel} | {views} | {duration} | {url} |

## Top video summary
**{title}** by {channel} ({views} views)
{transcript summary — 3-5 key points}
```

## Comprehensive research report

```
# Research Report: {topic}

## Web findings
{sin_web_search + websearch__websearch_search results}

## Video findings
{youtube__youtube_search + transcript results}

## Key takeaways
1. {finding}
2. {finding}
3. {finding}

## Sources
- [Web] {url}
- [YouTube] {url}
- [Reddit] {url}
```
