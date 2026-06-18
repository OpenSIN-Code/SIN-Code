# arXiv Search Strategy

1. Use the arXiv API:
   `http://export.arxiv.org/api/query?search_query=all:<query>&start=0&max_results=<n>&sortBy=relevance&sortOrder=descending`
2. Parse Atom XML. Extract title, summary, authors, published date, and primary category.
3. Cite with `https://arxiv.org/abs/<id>`.
4. If the user asks for the PDF, offer `https://arxiv.org/pdf/<id>.pdf`.
5. Use the official arXiv API; do not scrape HTML listings.
