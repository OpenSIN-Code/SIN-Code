// SPDX-License-Identifier: MIT
// Purpose: doc.md for internal/spec package.
// Spec Layer documentation and design notes.
// Docs: internal/spec/doc.md
package spec

/*
Spec Layer (Spectr) — Core Backbone

The Spec Layer provides a deterministic, markdown-based specification system
for encoding all project requirements, architecture decisions, and quality gates.

### Design Principles

1. **Deterministic**: No LLM calls or randomness. All operations are synchronous
   and produce identical output for identical input.

2. **Markdown-first**: Specs are stored as markdown + JSON metadata. Human-readable
   and version-control friendly.

3. **Immutable semantics**: Specs are never edited in-place. Mutations produce new
   Spec instances, enabling clean versioning and rollback.

4. **Non-breaking**: The Spec Layer is an additive feature. Existing Agent Loop
   workflows continue unchanged. Specs are "opt-in" enhancements.

5. **Lightweight**: No external dependencies beyond stdlib. Compiles to a small
   binary footprint.

### Core Components

#### types.go
- `Spec`: Core immutable spec container
- `SpecKind`: Enum for spec types (goal, process, constraint, component, integration)
- `SpecStatus`: Lifecycle states (draft, active, archived)
- `SpecCollection`: Set of related specs with dependency graph
- `DependencyGraph`: Directed acyclic graph (DAG) of spec dependencies

#### validate.go
- `ValidateSpec()`: Check single spec for required fields, markdown syntax
- `ValidateDependencies()`: Detect cycles, missing references in collection
- `ValidateTokenBudget()`: Ensure total token estimate stays within budget

#### merge.go
- `MergeSpecs()`: Three-way merge of specs (base, ours, theirs)
- `MergeStrategy`: Enum for conflict resolution (ours, theirs, newest, manual)
- `MergeConflict`: Field-level conflict details
- `MergeResult`: Merge outcome with resolved/unresolved conflicts

#### compiler.go (Phase 2)
- `CompileSpec()`: Validate and prepare spec for execution
- `DependencyGraph`: Build topological sort of specs
- `ComputeMetadata()`: Calculate static metadata (depth, hash)

#### gates.go (Phase 3)
- `Gate`: Abstract quality gate interface
- `TokenBudgetGate`: Verify token estimates
- `MarkdownSyntaxGate`: Check markdown formatting
- `DependencyGate`: Verify DAG structure

#### metaspec.go (Phase 4)
- `MetaSpec`: Compressed spec index for token optimization
- `SpecIndexer`: Build searchable index of specs
- `TokenBudgeter`: Allocate token budgets across specs

### Usage Example

```go
// Create spec
s := spec.NewSpec("spec_auth_001", "User Authentication System", spec.SpecKindGoal)
s.Description = "# Auth System\n..."
s.Goals = "- Support email+password login\n- Support OAuth\n..."
s.Constraints = "- No password reuse\n- Secure hashing required\n..."

// Validate
result := spec.ValidateSpec(s)
if !result.Valid {
    fmt.Println(result.Details())
    return
}

// Store in collection
collection := spec.NewCollection("root", "My Project")
collection.AddSpec(s)

// Build dependency graph
compiler := spec.NewCompiler(collection)
if err := compiler.BuildGraph(); err != nil {
    return err
}

// Export to markdown
fmt.Println(s.MarkdownFormat())
```

### File Layout

```
.sin/
├── specs/
│   ├── collection.json          # Metadata + statistics
│   ├── active/                  # Active specs
│   │   └── spec_auth_001.json
│   ├── drafts/                  # Draft specs
│   │   └── spec_payment_draft.json
│   └── archive/                 # Archived specs
│       └── spec_old_v1.json
```

### CLI Commands

```bash
sin-code spec init              # Initialize collection
sin-code spec create            # Create new spec
sin-code spec validate          # Validate all specs
sin-code spec archive           # Archive spec
sin-code spec list              # List all specs
sin-code spec show <id>         # Display spec
sin-code spec merge <base> <ours> <theirs>  # Three-way merge
```

### Future Work

**Phase 2 (SpecD - Compiler)**:
- Dependency graph validation
- Topological sorting
- Static analysis passes

**Phase 3 (SDLC - Quality Gates)**:
- Gate framework + built-in gates
- Integration with Agent Loop verification
- Test case generation from specs

**Phase 4 (MetaSpec - Token Optimization)**:
- Spec indexing and summarization
- Token budget allocation
- Dynamic spec selection for context window

**Phase 5 (SpecKit - UI/Commands)**:
- Chat slash-commands (/spec, /goal, etc)
- YAML-based command definitions
- Agent-spec interaction patterns
*/
