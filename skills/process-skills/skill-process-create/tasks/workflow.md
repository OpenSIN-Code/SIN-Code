# Workflow

## Step 1: Gather Intent

Ask the user (or clarify for yourself):

- What is the skill supposed to do?
- What trigger phrases should activate it?
- Is it a bundled repository skill or a local user skill?
- Does it depend on specific SIN tools?

## Step 2: Choose Category and Name

Use the bundled skill categories table. Pick a name that is:

- Kebab-case.
- Descriptive.
- Unique in the chosen category.

If the target category already has a `skill-<category>-create` name, fall back to the next logical category (e.g., `process` instead of `code`).

## Step 3: Create Structure

Run the equivalent of:

```bash
mkdir -p skills/<category>-skills/<skill-name>/{context,frameworks,tasks,templates}
touch skills/<category>-skills/<skill-name>/SKILL.md
touch skills/<category>-skills/<skill-name>/LICENSE
```

## Step 4: Write SKILL.md

Include:

- YAML frontmatter with `name`, `description`, `license`, `compatibility`, `metadata`, and `lifecycle`.
- An overview section.
- When to use / when not to use.
- Core process with a diagram or numbered list.
- Structure example.
- Naming and lifecycle rules.
- Verification checklist.

## Step 5: Fill Subdirectories

Add at least one `.md` file to each of `context/`, `frameworks/`, `tasks/`, `templates/`.

## Step 6: Validate

```bash
python3 scripts/validate_skill.py skills/<category>-skills/<skill-name> --strict
python3 scripts/validate_skill.py --all-bundled --strict
```

## Step 7: Build and Test

```bash
go build ./cmd/sin-code/...
go test ./cmd/sin-code -race -count=1
```

## Step 8: Commit

Use a conventional commit:

```bash
git add skills/<category>-skills/<skill-name>/
git commit -m "feat: bundle <skill-name> into repository"
```

## Step 9: Push

Push to `main` only after validation and tests pass.
