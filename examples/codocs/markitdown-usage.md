# MarkItDown Usage Example

## Basic CLI

```bash
# PDF → Markdown
markitdown spec.pdf > spec.md
# spec.md becomes spec.doc.md (CoDocs Standard)

# Office docs
markitdown report.docx -o report.doc.md
markitdown data.xlsx -o data.doc.md
markitdown slides.pptx -o slides.doc.md
```

## Python API + CoDocs

```python
# Docs: markitdown-usage.doc.md
from markitdown import MarkItDown
from pathlib import Path

def generate_docmd(source: str) -> str:
    """Convert any file to .doc.md format."""
    md = MarkItDown()
    result = md.convert(source)
    out_path = Path(source).with_suffix(".doc.md")
    out_path.write_text(result.text_content)
    return str(out_path)

# Usage
doc = generate_docmd("api-spec.pdf")
# → api-spec.doc.md ready for # Docs: reference
```

## Pipeline: PDF → CoDocs

```bash
for f in docs/*.pdf; do
    markitdown "$f" -o "${f%.pdf}.doc.md"
    echo "Created ${f%.pdf}.doc.md"
done
```
