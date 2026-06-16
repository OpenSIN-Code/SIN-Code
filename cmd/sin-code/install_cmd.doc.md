# `sin-code install` — single-binary installer entrypoint

Issue **#170** (`feat(install): one-line curl|bash installer`).
Mirrors the caveman install pattern on top of the existing
goreleaser-style release pipeline (M2: single static Go binary,
CGO_ENABLED=0).

## Surface

```bash
curl -fsSL https://raw.githubusercontent.com/OpenSIN-Code/SIN-Code/main/install.sh | bash
# PowerShell:
irm https://raw.githubusercontent.com/OpenSIN-Code/SIN-Code/main/install.ps1 | iex
```

Both shims delegate to:

```bash
sin-code install [--auto]
```

| Flag            | Default                | Meaning                                     |
| --------------- | ---------------------- | ------------------------------------------- |
| `--auto`        | `false` (interactive) | accept all defaults, no prompts             |
| `--dir <path>`  | `$SIN_CODE_BIN_DIR` / `~/.local/bin` | install destination           |
| `--release <v>` | `latest`               | pin a specific tag (CI reproducibility)     |
| `--channel`     | `stable`               | advisory; `dev` is future rolling-tip       |
| `--no-verify`   | `false`                | skip SHA256 (offline / sanctioned CI only)  |
| `--dry-run`     | `false`                | print plan + URLs, touch nothing            |
| `--verify-only` | `false`                | health-check `$dir/sin-code` without overwrite |

## Internals

The Go entrypoint lives in `cmd/sin-code/internal/install/`:

| File              | Responsibility                                              |
| ----------------- | ----------------------------------------------------------- |
| `release.go`      | canonical Repo / URLs, `Release` + `Asset` types, asset naming |
| `github.go`       | pure-stdlib REST client (no `gh`, no `jq` — bootstrap safe)  |
| `verify.go`       | streaming SHA256 + `checksums.txt` parser + opt-in cosign    |
| `composer.go`     | tar.gz / zip extraction, atomic placement, `ChooseBinDir`   |

The 1031-line legacy `install.sh` is replaced by a 30-line shim at
the repo root that downloads the archive, extracts the binary, and
`exec`s `sin-code install --auto`. The legacy 12-step flow
(Python bundle, 7 Go tools, opencode.json patches) is permanently
retired: post-v3.0 the `sin-code` binary IS the unified tool
(7 tool subcommands ship as `sin-code discover`, `sin-code
execute`, …), so the legacy flow was already duplicative.

## Hard-mandate adherence

- **M2 (single static binary)** — installer downloads a tarball
  containing ONE Go binary + LICENSE/README/SECURITY provenance
  files. The installed artifact is exactly `sin-code` (or
  `sin-code.exe` on Windows); no shell scripts, no vendored Python,
  no extra binaries.
- **M3 (verify-gate)** — `--verify-only` short-circuits on missing
  binary; SHA256 verification step is mandatory when checksums.txt
  is reachable (fail-closed).
- **M4 (permission engine)** — `install__download`, `install__place`
  semantically belong to the agent loop but resolve to filesystem
  writes; the cobra RunE executes them under the same headless
  policy. No silent sudo, no privilege escalation.
- **M5 (module path)** — every import in this package uses
  `github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/install`.
  No `SIN-Code-Bundle` references.
- **M6 (sandbox-friendly)** — every external dependency (`curl`,
  `tar`, `unzip`) is documented at the top of the shims. No
  npm / pipx in the bootstrap path.
- **M7 (race-free)** — package passes `go test -race -count=1`.

## Deliberate non-features

- No GPG signature step at install time. The goreleaser cosign
  signature covers `checksums.txt` (§ `.goreleaser.yaml` line 56)
  but every release also embeds a SHA256 in the GitHub release page
  itself, which is the line of defence offline installs rely on.
  Cosign verification is wired in `verify.go`; turning it on the
  default path is a follow-up issue once the project stabilizes its
  keyless signing pipeline.
- No progress bars. Silence is faster than spurious output on slow
  cellular — the shims print exactly three lines.
- No PATH manipulation in the shim itself. The hint is a
  printf-able export line the user pastes into their shell rc.
