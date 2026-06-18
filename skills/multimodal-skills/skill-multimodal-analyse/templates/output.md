# Template: Output Format

Docs: ../SKILL.md

## Modality Header

```markdown
## Analyse: {file_path}
**Tool:** `analyse__{tool}`
**Modality:** {image|PDF|log|data|audio|video}
**Verified at:** {ISO timestamp}
```

## Evidence Block

```markdown
### Extracted Evidence
[verbatim quote or summarised table from `analyse__*` output]
```

## Conclusion

```markdown
### Conclusion
{Reasoning grounded in the evidence block above.}

### Citation
[analyse__{tool} {file_path}, {result_locator}]
```
