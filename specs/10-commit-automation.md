# Commit Automation Specification

**Project**: gzh-cli-git
**Feature**: Commit Automation (F1)
**Phase**: Phase 2
**Version**: 1.0
**Last Updated**: 2025-11-27
**Status**: Draft
**Priority**: P0 (High)

______________________________________________________________________

## 1. Overview

### 1.1 Purpose

This specification defines the commit automation features for gzh-cli-git, including template-based commits, auto-generated commit messages, message validation, and smart push functionality.

### 1.2 Goals

- **Consistency**: Ensure commit messages follow team conventions
- **Efficiency**: Reduce time spent writing commit messages (30% time savings)
- **Quality**: Validate commit messages against standards
- **Safety**: Prevent accidental destructive operations

### 1.3 Non-Goals

- GUI commit tools (CLI only)
- Git hook management (deferred to future)
- Commit message translation (out of scope)

______________________________________________________________________

## 2. Requirements

### 2.1 Functional Requirements

**FR-1**: Template System

- Support built-in templates (Conventional Commits, Semantic Versioning)
- Support custom user templates
- Template validation
- Template inheritance/composition

**FR-2**: Auto-Commit

- Generate commit messages from git diff
- Analyze changed files to suggest type and scope
- Support customization of auto-generation logic
- Preview before committing

**FR-3**: Smart Push

- Safety checks before push
- Prevent force push to protected branches
- Detect diverged branches
- Confirmation for risky operations

**FR-4**: Message Validation

- Validate against template rules
- Check message length limits
- Detect common mistakes (typos, formatting)
- Provide helpful error messages

### 2.2 Non-Functional Requirements

**NFR-1**: Performance

- Template loading: \<10ms
- Commit creation: \<100ms
- Validation: \<50ms

**NFR-2**: Usability

- Intuitive CLI commands
- Clear error messages
- Good defaults (minimal config needed)

**NFR-3**: Compatibility

- Git 2.30+
- Cross-platform (Linux, macOS, Windows)

______________________________________________________________________

## 3. Design

### 3.1 Architecture

```
┌─────────────────────────────────────────────┐
│           CLI Layer (cmd/)                  │
│  ┌─────────────┐  ┌──────────────────────┐  │
│  │ commit cmd  │  │ push cmd            │  │
│  └─────────────┘  └──────────────────────┘  │
└──────────────┬─────────────┬────────────────┘
               │             │
         ┌─────▼─────────────▼─────────────┐
         │  Library Layer (pkg/commit)     │
         │  ┌────────────┐  ┌────────────┐ │
         │  │ Template   │  │ Validator  │ │
         │  │ Manager    │  │            │ │
         │  └────────────┘  └────────────┘ │
         │  ┌────────────┐  ┌────────────┐ │
         │  │ Auto-Gen   │  │ Smart Push │ │
         │  │            │  │            │ │
         │  └────────────┘  └────────────┘ │
         └─────────────────────────────────┘
                      │
         ┌────────────▼──────────────────┐
         │  Git Layer (internal/gitcmd)  │
         │  ┌────────────────────────┐   │
         │  │  Git Command Executor  │   │
         │  └────────────────────────┘   │
         └───────────────────────────────┘
```

### 3.2 Component Interfaces

#### Template Manager

```go
// TemplateManager manages commit message templates
type TemplateManager interface {
    // Load loads a template by name
    Load(ctx context.Context, name string) (*Template, error)

    // LoadCustom loads a custom template from file
    LoadCustom(ctx context.Context, path string) (*Template, error)

    // List returns available template names
    List(ctx context.Context) ([]string, error)

    // Validate validates a template
    Validate(ctx context.Context, template *Template) error
}

// Template represents a commit message template
type Template struct {
    Name        string
    Description string
    Format      string  // Go template format
    Rules       []ValidationRule
    Examples    []string
    Variables   []TemplateVariable
}

// TemplateVariable defines a template variable
type TemplateVariable struct {
    Name        string
    Type        string  // string, enum, bool
    Required    bool
    Default     string
    Options     []string  // for enum types
    Description string
}

// ValidationRule defines message validation
type ValidationRule struct {
    Type    string  // length, pattern, required
    Pattern string  // regex for pattern rules
    Message string  // error message
}
```

#### Auto-Commit Generator

```go
// Generator generates commit messages automatically
type Generator interface {
    // Generate creates a commit message from changes
    Generate(ctx context.Context, repo *repository.Repository, opts GenerateOptions) (string, error)

    // Suggest suggests commit type and scope
    Suggest(ctx context.Context, changes *DiffSummary) (*Suggestion, error)
}

// GenerateOptions configures message generation
type GenerateOptions struct {
    Template    *Template
    Interactive bool  // Ask user for clarifications
    MaxLength   int
}

// DiffSummary summarizes git diff
type DiffSummary struct {
    FilesChanged  int
    Insertions    int
    Deletions     int
    ModifiedFiles []string
    AddedFiles    []string
    DeletedFiles  []string
}

// Suggestion suggests commit metadata
type Suggestion struct {
    Type        string  // feat, fix, docs, etc.
    Scope       string  // Inferred scope
    Description string  // Generated description
    Confidence  float64 // 0.0 - 1.0
}
```

#### Validator

```go
// Validator validates commit messages
type Validator interface {
    // Validate checks if message follows template rules
    Validate(ctx context.Context, message string, template *Template) (*ValidationResult, error)

    // ValidateInteractive validates with user interaction
    ValidateInteractive(ctx context.Context, message string) (*ValidationResult, error)
}

// ValidationResult contains validation results
type ValidationResult struct {
    Valid    bool
    Errors   []ValidationError
    Warnings []ValidationWarning
}

// ValidationError represents a validation error
type ValidationError struct {
    Rule    string
    Message string
    Line    int
    Column  int
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
    Message string
    Suggestion string
}
```

#### Smart Push

```go
// SmartPush provides safe push operations
type SmartPush interface {
    // Push performs a safe push with checks
    Push(ctx context.Context, repo *repository.Repository, opts PushOptions) error

    // CanPush checks if push is safe
    CanPush(ctx context.Context, repo *repository.Repository) (*PushCheck, error)
}

// PushOptions configures push behavior
type PushOptions struct {
    Remote       string
    Branch       string
    Force        bool
    SetUpstream  bool
    DryRun       bool
    SkipChecks   bool  // For emergency use
}

// PushCheck contains push safety check results
type PushCheck struct {
    Safe            bool
    Issues          []PushIssue
    Recommendations []string
}

// PushIssue represents a push safety issue
type PushIssue struct {
    Severity string  // error, warning, info
    Message  string
    Blocker  bool    // Blocks push if true
}
```

### 3.3 Data Structures

#### Built-in Templates

**Conventional Commits Template**:

```yaml
name: conventional
description: Conventional Commits 1.0.0
format: |
  {{.Type}}{{if .Scope}}({{.Scope}}){{end}}: {{.Description}}
  {{if .Body}}
  {{.Body}}
  {{end}}
  {{if .Footer}}
  {{.Footer}}
  {{end}}
variables:
  - name: Type
    type: enum
    required: true
    options: [feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert]
    description: Type of change
  - name: Scope
    type: string
    required: false
    description: Scope of change (e.g., component, file)
  - name: Description
    type: string
    required: true
    description: Short description of change
  - name: Body
    type: string
    required: false
    description: Detailed description
  - name: Footer
    type: string
    required: false
    description: Footer (breaking changes, references)
rules:
  - type: length
    pattern: "^.{1,72}$"
    message: "Subject line must be 1-72 characters"
  - type: pattern
    pattern: "^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\\(.+\\))?: .+"
    message: "Must follow Conventional Commits format"
examples:
  - "feat(cli): add commit automation command"
  - "fix(parser): handle empty git status output"
  - "docs: update README with new examples"
```

**Semantic Versioning Template**:

```yaml
name: semantic
description: Semantic Versioning commit template
format: |
  [{{.Category}}] {{.Description}}
  {{if .Details}}
  Details: {{.Details}}
  {{end}}
  Affects: {{.Version}}
variables:
  - name: Category
    type: enum
    required: true
    options: [MAJOR, MINOR, PATCH, BUILD]
    description: Semver category
  - name: Description
    type: string
    required: true
  - name: Details
    type: string
    required: false
  - name: Version
    type: string
    required: true
    default: "patch"
rules:
  - type: length
    message: "Description must be 1-100 characters"
examples:
  - "[MINOR] Add new API endpoint"
  - "[PATCH] Fix typo in error message"
```

______________________________________________________________________

## 4. Implementation

### 4.1 Key Components

**pkg/commit/template.go**

- Template loading from built-in and custom sources
- Template validation
- Template variable substitution

**pkg/commit/generator.go**

- Analyze git diff to suggest commit type/scope
- Generate commit description from changes
- Support for custom generation rules

**pkg/commit/validator.go**

- Validate message against template rules
- Check message length, format, content
- Provide actionable error messages

**pkg/commit/push.go**

- Pre-push safety checks
- Protected branch detection
- Force push prevention
- Diverged branch handling

### 4.2 Dependencies

- `pkg/repository` - Repository operations
- `internal/gitcmd` - Git command execution
- `internal/parser` - Git output parsing
- `text/template` - Go template engine

### 4.3 Error Handling

```go
// Commit errors
var (
    ErrTemplateNotFound = errors.New("template not found")
    ErrInvalidTemplate  = errors.New("invalid template format")
    ErrValidationFailed = errors.New("message validation failed")
    ErrPushBlocked      = errors.New("push blocked by safety check")
    ErrNoChanges        = errors.New("no changes to commit")
)

// CommitError provides rich error context
type CommitError struct {
    Op      string  // Operation (load, validate, push)
    Cause   error
    Message string
    Hints   []string  // Suggestions to fix
}
```

______________________________________________________________________

## 5. Testing

### 5.1 Test Scenarios

**Template System**:

- Load built-in templates (conventional, semantic)
- Load custom templates from file
- Validate template format
- Variable substitution
- Template inheritance
- Invalid template handling

**Auto-Generation**:

- Generate message from simple changes
- Generate message from complex changes
- Suggest type from file patterns
- Suggest scope from directory structure
- Handle empty diff
- Handle large diffs

**Validation**:

- Validate conventional commits format
- Validate semantic commits format
- Check message length limits
- Detect common typos
- Provide helpful suggestions
- Interactive validation

**Smart Push**:

- Allow push to non-protected branches
- Block force push to main/master
- Detect diverged branches
- Handle upstream not set
- Dry-run mode
- Emergency override

### 5.2 Coverage Requirements

- Template Manager: ≥90%
- Generator: ≥85%
- Validator: ≥90%
- Smart Push: ≥85%

### 5.3 Edge Cases

- Empty commit messages
- Very long commit messages (>10KB)
- Non-UTF8 characters
- Templates with circular dependencies
- Concurrent template modifications
- Network failures during push

______________________________________________________________________

## 6. Examples

### 6.1 CLI Usage

**Using Built-in Template**:

```bash
# Interactive template-based commit
gzh-git commit --template conventional

# Non-interactive with all options
gzh-git commit --template conventional \
  --type feat \
  --scope cli \
  --message "add commit automation" \
  --body "Implements template system and auto-generation"

# Auto-generate commit message
gzh-git commit --auto

# Auto-generate with preview
gzh-git commit --auto --dry-run
```

**Using Custom Template**:

```bash
# Load from file
gzh-git commit --template-file ~/.config/gzh-git/my-template.yaml

# Set as default
gzh-git config template.default my-template
```

**Smart Push**:

```bash
# Safe push with checks
gzh-git push --smart

# Dry-run to see what would happen
gzh-git push --smart --dry-run

# Override safety checks (emergency)
gzh-git push --smart --force --skip-checks
```

**Message Validation**:

```bash
# Validate message from file
gzh-git commit --validate --message-file commit.txt

# Validate with specific template
gzh-git commit --validate --template conventional \
  --message "feat: add feature"
```

### 6.2 Library Usage

**Template-Based Commit**:

```go
package main

import (
    "context"
    "github.com/gizzahub/gzh-cli-git/pkg/commit"
    "github.com/gizzahub/gzh-cli-git/pkg/repository"
)

func main() {
    ctx := context.Background()

    // Open repository
    repoClient := repository.NewClient()
    repo, _ := repoClient.Open(ctx, ".")

    // Load template
    tmplMgr := commit.NewTemplateManager()
    tmpl, _ := tmplMgr.Load(ctx, "conventional")

    // Create commit
    commitMgr := commit.NewManager()
    result, _ := commitMgr.CreateFromTemplate(ctx, repo, commit.TemplateOptions{
        Template: tmpl,
        Values: map[string]string{
            "Type":        "feat",
            "Scope":       "api",
            "Description": "add new endpoint",
        },
    })

    println("Commit created:", result.Hash)
}
```

**Auto-Generated Commit**:

```go
func autoCommit() {
    ctx := context.Background()

    // Initialize
    repoClient := repository.NewClient()
    repo, _ := repoClient.Open(ctx, ".")

    generator := commit.NewGenerator()
    commitMgr := commit.NewManager()

    // Generate message
    message, _ := generator.Generate(ctx, repo, commit.GenerateOptions{
        Interactive: false,
    })

    // Create commit
    result, _ := commitMgr.Create(ctx, repo, commit.CommitOptions{
        Message: message,
    })

    println("Auto-commit created:", result.Hash)
}
```

**Smart Push**:

```go
func smartPush() {
    ctx := context.Background()

    repoClient := repository.NewClient()
    repo, _ := repoClient.Open(ctx, ".")

    pusher := commit.NewSmartPush()

    // Check if safe to push
    check, _ := pusher.CanPush(ctx, repo)
    if !check.Safe {
        for _, issue := range check.Issues {
            println("Issue:", issue.Message)
        }
        return
    }

    // Perform safe push
    _ = pusher.Push(ctx, repo, commit.PushOptions{
        Remote: "origin",
        Branch: "main",
    })
}
```

______________________________________________________________________

## 7. Acceptance Criteria

### 7.1 Feature Completeness

- [ ] Built-in templates (conventional, semantic) working
- [ ] Custom template loading from file
- [ ] Template validation
- [ ] Auto-message generation from diff
- [ ] Type/scope suggestion logic
- [ ] Message validation against template
- [ ] Smart push with safety checks
- [ ] CLI commands implemented
- [ ] Library APIs implemented

### 7.2 Quality Gates

- [ ] Test coverage ≥85%
- [ ] All linters passing
- [ ] Documentation complete (GoDoc)
- [ ] Examples working
- [ ] Integration tests passing

### 7.3 User Validation

- [ ] Alpha users can create commits with templates
- [ ] Auto-generated messages are helpful (≥80% acceptance)
- [ ] Smart push prevents accidents (zero force-push incidents)
- [ ] Template system is intuitive (≤5 min to create custom template)

______________________________________________________________________

## 8. Risks & Mitigation

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Template syntax too complex | Medium | Medium | Provide wizard for template creation |
| Auto-gen messages inaccurate | Medium | High | Allow editing before commit; improve algorithms |
| Smart push too restrictive | Low | Medium | Provide override flag; clear error messages |
| Performance with large diffs | Low | Low | Implement diff size limits; optimize parsing |

______________________________________________________________________

## 9. Open Questions

1. Should templates support hooks/plugins for custom logic?
1. How to handle merge commits in auto-generation?
1. Support for GPG signing in smart push?
1. Should we provide a template marketplace/registry?

______________________________________________________________________

## 10. References

### 10.1 Standards

- [Conventional Commits 1.0.0](https://www.conventionalcommits.org/)
- [Semantic Versioning 2.0.0](https://semver.org/)

### 10.2 Similar Tools

- commitizen (interactive commit tool)
- commitlint (commit message linter)
- husky (Git hooks manager)

### 10.3 Internal Documents

- [PRD.md](../PRD.md) - Section 4.2
- [ARCHITECTURE.md](../ARCHITECTURE.md) - Section 5
- [specs/00-overview.md](./00-overview.md)

______________________________________________________________________

**Approval Required From**:

- [ ] Product Owner
- [ ] Tech Lead
- [ ] Security Team (for push safety features)

**Next Steps**:

1. Review and approve specification
1. Create implementation tasks
1. Begin Phase 2 development

______________________________________________________________________

**Document Version**: 1.0
**Status**: Draft - Pending Review
**Last Updated**: 2025-11-27
