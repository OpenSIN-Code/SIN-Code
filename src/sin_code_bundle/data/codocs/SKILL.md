---
name: sin-codocs
description: Co-located Docs Standard — every code file has a `.doc.md` companion. Create it for new files, reference it on changes, check for broken links with `sin codocs check`.
---

## CoDocs Standard

Every code file gets a `.doc.md` companion file in the same directory.

### Naming

```
router.py         → router.doc.md
config.yaml       → config.doc.md
api/types.ts      → api/types.doc.md
Makefile          → Makefile.doc.md
```

### Code reference

First line of the code file:

```python
# Docs: router.doc.md
```

```ts
// Docs: types.doc.md
```

```makefile
# Docs: Makefile.doc.md
```

### What belongs in a `.doc.md`

- What does this file do? (1 sentence)
- Which other files import / touch it?
- Important config values & limits
- Why certain decisions were made (e.g. "no async here because X")

### What does NOT belong in a `.doc.md`

- Implementation details (the code speaks for itself)
- Git history (that's what `git log` is for)

### Validation

After changes, verify references with the bundle CLI (robust, replaces the old
`grep | sed` one-liner):

```bash
sin codocs check            # exit 1 if any reference is broken
sin codocs check --json     # machine-readable output
sin codocs list             # list every reference and whether it resolves
```

## Exceptions

- `docs/` folder for architecture docs, ADRs, setup guides
- `README.md` for project overview
- NO `.doc.md` for pure config files without logic (`.gitignore`, `.prettierrc`, etc.)

---

## MarkItDown Integration (Microsoft)

**Converts everything to Markdown** for LLM consumption: PDF, DOCX, PPTX, XLSX,
Images (OCR), Audio, HTML, CSV/JSON/XML, ZIP, YouTube, EPUB, Outlook MSG.

### Installation

```bash
pipx install markitdown                          # recommended (CLI + library)
pip install 'markitdown[pdf, docx, pptx, xlsx]'  # minimal
pip install 'markitdown[all]'                    # everything
```

### CLI

```bash
markitdown file.pdf > file.md                     # stdout
markitdown file.pdf -o file.md                    # output file
cat file.pdf | markitdown                         # pipe
markitdown --use-plugins file.pdf                 # with plugins (OCR)
markitdown file.pdf --use-cu --cu-endpoint "<e>"  # Azure Content Understanding
```

### Python API

```python
from markitdown import MarkItDown
md = MarkItDown()
result = md.convert("document.pdf")
print(result.text_content)

# With LLM vision (image descriptions in PPTX/Images)
from openai import OpenAI
md = MarkItDown(llm_client=OpenAI(), llm_model="gpt-4o")

# Security: local files only
result = md.convert_local("document.pdf")
```

### CoDocs pipeline

```bash
for f in docs/*.pdf docs/*.docx docs/*.pptx; do
    markitdown "$f" -o "${f%.*}.doc.md"
done
# then add `# Docs: filename.doc.md` to the matching code file
```

### Security

- `convert()` runs with the calling process's full file-IO rights. Never pass
  untrusted input directly.
- Prefer `convert_local()` / `convert_stream()` for controlled access.

### Reference

https://github.com/microsoft/markitdown | `pipx install markitdown`
