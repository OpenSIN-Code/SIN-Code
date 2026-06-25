# Delegation Prompt Template — Code Implementation Task

Use this template when delegating code implementation to a `general` subagent.

## Template

```
You are working in the [PROJECT NAME] repository at [ABSOLUTE PATH].

Your task: [ONE SENTENCE OBJECTIVE — what to build/fix/add]

## Context
- Project: [language, framework, what it does]
- Module path: [e.g. github.com/OpenSIN-Code/SIN-Code]
- Relevant files: [list 3-5 key file paths the subagent should read first]
- Conventions: [coding style, test framework, lint tool, build command]
- Existing patterns: [how similar features are implemented — reference a specific file]
- Dependencies: [what packages/modules are already available, what NOT to add]

## What to do
[DETAILED step-by-step instructions. Be specific about:]
1. [First step — what to read/understand]
2. [Second step — what to create/modify]
3. [Third step — what to test]

## Constraints
- Do NOT [edit file X — it's owned by another subagent]
- Do NOT [add dependency Y — not approved]
- Do NOT [change API Z — public interface]
- Do NOT [commit anything — just write files]
- Do NOT [edit AGENTS.md or CHANGELOG.md]
- Follow [specific convention: e.g. "use cobra for CLI commands"]
- Match [existing code style — look at neighboring files]

## Output
- Write your implementation to: [EXACT file path]
- Write your tests to: [EXACT file path]
- Return a summary of: [what you created, what you tested, any issues]

## Verification
Before returning, verify your work:
1. Run: [build command, e.g. "go build ./..."]
2. Run: [test command, e.g. "go test -race -count=1 ./cmd/sin-code/..."]
3. Run: [lint command, e.g. "golangci-lint run ./..."]
4. Fix any issues found before returning

## Important
- Do NOT commit anything. Just write the files.
- Do NOT edit AGENTS.md or CHANGELOG.md.
- Report back: [list of files created/modified, build/test/lint status, any issues]
```

## Example (filled in)

```
You are working in the SIN-Code repository at /Users/jeremy/dev/SIN-Code-Bundle.

Your task: Implement a `sin-code doctor` subcommand — unified health check.

## Context
- Project: Go 1.23+, cobra-based CLI, single binary (CGO_ENABLED=0)
- Module path: github.com/OpenSIN-Code/SIN-Code
- Relevant files: cmd/sin-code/main.go (command registration),
  cmd/sin-code/status_cmd.go (similar pattern to follow),
  cmd/sin-code/stack_cmd.go (another reference for doctor pattern)
- Conventions: cobra commands, gofmt, golangci-lint, go test -race
- Existing patterns: status_cmd.go uses --json flag, renders table output

## What to do
1. Read status_cmd.go and stack_cmd.go to understand the command pattern
2. Create cmd/sin-code/doctor_cmd.go with NewDoctorCmd()
3. Implement 11 health checks (Go toolchain, binary, config, DBs, MCP, tools, module, CGO)
4. Support --json and --quiet flags, exit 1 on FAIL
5. Create doctor_cmd_test.go with tests for each check function

## Constraints
- Do NOT edit main.go beyond adding AddCommand line
- Do NOT add external dependencies — stdlib + cobra only
- Do NOT commit anything
- Follow the cobra pattern from status_cmd.go

## Output
- Write implementation to: cmd/sin-code/doctor_cmd.go
- Write tests to: cmd/sin-code/doctor_cmd_test.go

## Verification
1. Run: go build ./...
2. Run: go vet ./cmd/sin-code/...
3. Run: go test -race -count=1 ./cmd/sin-code/...

## Important
- Do NOT commit. Just write files.
- Report back: files created, build/test status, any issues found.
```
