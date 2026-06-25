# Task: Parallel Feature Implementation

## When to use
When implementing multiple independent features that can be built simultaneously.

## Pattern
1. Identify N independent features
2. Assign non-overlapping file sets to each subagent
3. Launch all N subagents in ONE message
4. Wait for all to complete
5. Run integration verification

## Example: 4 New Subcommands

### Subagent A: doctor command
Files: cmd/sin-code/doctor_cmd.go, cmd/sin-code/doctor_cmd_test.go
Model: general (needs write access)
Prompt: [full delegation prompt for doctor]

### Subagent B: diff command
Files: cmd/sin-code/diff_cmd.go, cmd/sin-code/diff_cmd_test.go
Model: general
Prompt: [full delegation prompt for diff]

### Subagent C: benchmark command
Files: cmd/sin-code/benchmark_cmd.go, cmd/sin-code/benchmark_cmd_test.go
Model: general
Prompt: [full delegation prompt for benchmark]

### Subagent D: tokens cost command
Files: cmd/sin-code/tokens_cost_cmd.go, cmd/sin-code/tokens_cost_cmd_test.go
Model: general
Prompt: [full delegation prompt for tokens cost]

### After all complete:
1. Update cmd/sin-code/main.go to register all 4 commands
2. Update cmd/sin-code/testdata/golden/help.golden
3. Run: go build ./... && golangci-lint run ./... && go test -race -count=1 ./cmd/sin-code/...
4. Commit and push
