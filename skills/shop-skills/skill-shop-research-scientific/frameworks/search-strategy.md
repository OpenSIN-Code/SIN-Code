# Search Strategy Framework

- Start with the broadest public API endpoint.
- Limit results to the top 5–10 most relevant items.
- For each item, capture: source, title, URL, date, and a one-line summary.
- Synthesize results in Markdown with a bulleted list.
- If no results are found, explicitly state "No results found for <query>" and suggest broadening the query.
- Never cite URLs you did not fetch.
