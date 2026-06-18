# PubMed Search Strategy

1. Build a query with MeSH terms when possible; otherwise use plain keywords.
2. Use the NCBI E-utilities URL:
   `https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi?db=pubmed&term=<query>&retmode=json&retmax=<n>`
3. Fetch summaries for PMIDs:
   `https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esummary.fcgi?db=pubmed&id=<pmid>&retmode=json`
4. Cite each result with `https://pubmed.ncbi.nlm.nih.gov/<pmid>/`.
5. Respect rate limits: no more than 3 requests/second without an API key.
