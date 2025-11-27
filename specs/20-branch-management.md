# Branch Management Specification

**Project**: gzh-cli-git
**Feature**: Branch Management (F2)
**Phase**: Phase 3
**Version**: 1.0
**Last Updated**: 2025-11-27
**Status**: Draft
**Priority**: P0 (High)

---

## 1. Overview

### 1.1 Purpose

This specification defines the branch management features for gzh-cli-git, including branch operations, worktree management, and parallel workflow support for efficient multi-context development.

### 1.2 Goals

- **Efficiency**: Simplify branch creation, deletion, and cleanup operations
- **Parallel Development**: Enable seamless multi-context work via worktrees
- **Safety**: Prevent accidental deletion of important branches
- **Productivity**: Reduce context-switching overhead by 60%

### 1.3 Non-Goals

- Git Flow or GitLab Flow automation (deferred to future)
- Branch protection rules configuration (deferred to future)
- Remote branch synchronization automation (basic support only)
- GUI branch visualization (CLI only)

---

## 2. Requirements

### 2.1 Functional Requirements

**FR-1**: Branch Creation
- Create new branches from any ref (commit, tag, branch)
- Support branch naming conventions validation
- Set upstream tracking automatically
- Create and checkout in one operation

**FR-2**: Branch Deletion
- Delete local branches with safety checks
- Delete remote branches with confirmation
- Detect unmerged branches before deletion
- Force delete with explicit flag

**FR-3**: Branch Cleanup
- Identify merged branches safe to delete
- Identify stale branches (no recent activity)
- Bulk delete with confirmation
- Preserve protected branches (main, master, develop, release/*)

**FR-4**: Worktree Management
- Add worktrees for parallel development
- Remove worktrees safely
- List all worktrees with status
- Track worktree-branch associations
- Clean up orphaned worktrees

**FR-5**: Parallel Workflows
- Switch between worktrees efficiently
- Share configuration across worktrees
- Independent operations per worktree
- Prevent conflicting operations

### 2.2 Non-Functional Requirements

**NFR-1**: Performance
- Branch creation: <50ms
- Branch deletion: <100ms
- Worktree add: <200ms
- Cleanup scan: <500ms for 100 branches

**NFR-2**: Usability
- Intuitive command names
- Clear confirmation prompts
- Helpful error messages
- Progress indicators for slow operations

**NFR-3**: Safety
- Prevent data loss
- Confirmation for destructive operations
- Dry-run mode for cleanup
- Undo capability where possible

---

## 3. Design

### 3.1 Architecture

```
┌─────────────────────────────────────────────┐
│           CLI Layer (cmd/)                  │
│  ┌─────────────┐  ┌──────────────────────┐  │
│  │ branch cmd  │  │ worktree cmd        │  │
│  └─────────────┘  └──────────────────────┘  │
└──────────────┬─────────────┬────────────────┘
               │             │
         ┌─────▼─────────────▼─────────────┐
         │  Library Layer (pkg/branch)     │
         │  ┌────────────┐  ┌────────────┐ │
         │  │ Branch Mgr │  │ Worktree   │ │
         │  │            │  │ Manager    │ │
         │  └────────────┘  └────────────┘ │
         │  ┌────────────┐  ┌────────────┐ │
         │  │ Cleanup    │  │ Parallel   │ │
         │  │ Service    │  │ Workflow   │ │
         │  └────────────┘  └────────────┘ │
         └──────────────┬──────────────────┘
                        │
         ┌──────────────▼──────────────────┐
         │     Foundation (internal/)      │
         │  ┌────────────┐  ┌────────────┐ │
         │  │ Git Exec   │  │ Parser     │ │
         │  └────────────┘  └────────────┘ │
         └─────────────────────────────────┘
```

### 3.2 Component Responsibilities

**Branch Manager (`pkg/branch/manager.go`)**:
- Core branch operations (create, delete, list)
- Branch naming validation
- Upstream tracking management
- Merge status detection

**Worktree Manager (`pkg/branch/worktree.go`)**:
- Worktree lifecycle (add, remove, list)
- Worktree validation and cleanup
- Path management
- Status tracking

**Cleanup Service (`pkg/branch/cleanup.go`)**:
- Branch analysis (merged, stale, orphaned)
- Cleanup strategy execution
- Safety checks and confirmations
- Reporting

**Parallel Workflow (`pkg/branch/parallel.go`)**:
- Multi-worktree coordination
- Conflict detection
- Operation synchronization
- Context switching helpers

---

## 4. Detailed Design

### 4.1 Branch Operations

#### 4.1.1 Branch Creation

**Interface**:
```go
type BranchManager interface {
    // Create creates a new branch
    Create(ctx context.Context, repo *repository.Repository, opts CreateOptions) error

    // Delete deletes a branch
    Delete(ctx context.Context, repo *repository.Repository, opts DeleteOptions) error

    // List lists branches
    List(ctx context.Context, repo *repository.Repository, opts ListOptions) ([]*Branch, error)
}

type CreateOptions struct {
    Name      string // Branch name (required)
    StartRef  string // Starting ref (default: HEAD)
    Checkout  bool   // Checkout after creation
    Track     bool   // Set upstream tracking
    Force     bool   // Overwrite existing branch
    Validate  bool   // Validate naming conventions
}

type Branch struct {
    Name      string
    Ref       string
    IsHead    bool
    IsMerged  bool
    Upstream  string
    LastCommit *Commit
}
```

**Validation Rules**:
- Branch name must match: `^[a-zA-Z0-9/_-]+$`
- Recommended formats:
  - `feature/{name}` - New features
  - `fix/{name}` - Bug fixes
  - `hotfix/{name}` - Urgent fixes
  - `release/{version}` - Release branches
  - `experiment/{name}` - Experimental work

**Error Handling**:
- `ErrBranchExists` - Branch already exists (use --force)
- `ErrInvalidName` - Branch name validation failed
- `ErrInvalidRef` - Starting ref doesn't exist
- `ErrDetachedHead` - Cannot create branch in detached HEAD state

#### 4.1.2 Branch Deletion

**Interface**:
```go
type DeleteOptions struct {
    Name     string   // Branch name (required)
    Remote   bool     // Delete remote branch
    Force    bool     // Force delete (even if unmerged)
    DryRun   bool     // Preview deletion
    Confirm  bool     // Skip confirmation prompt
}
```

**Safety Checks**:
1. Cannot delete current branch (must checkout another first)
2. Cannot delete protected branches without --force
3. Warn if branch is unmerged
4. Confirm if deleting remote branch

**Protected Branches** (default):
- `main`, `master`
- `develop`, `development`
- `release/*`, `hotfix/*`
- Configurable via `.gzh-git/config.yaml`

#### 4.1.3 Branch Cleanup

**Interface**:
```go
type CleanupService interface {
    // Analyze analyzes branches for cleanup
    Analyze(ctx context.Context, repo *repository.Repository, opts AnalyzeOptions) (*CleanupReport, error)

    // Execute performs cleanup based on strategy
    Execute(ctx context.Context, repo *repository.Repository, report *CleanupReport, opts ExecuteOptions) error
}

type AnalyzeOptions struct {
    IncludeMerged   bool      // Include fully merged branches
    IncludeStale    bool      // Include stale branches (no activity)
    StaleThreshold  time.Duration // Threshold for stale (default: 30 days)
    IncludeRemote   bool      // Include remote branches
    Exclude         []string  // Patterns to exclude
}

type CleanupReport struct {
    Merged    []*Branch  // Fully merged branches
    Stale     []*Branch  // Stale branches
    Orphaned  []*Branch  // Orphaned tracking branches
    Protected []*Branch  // Protected (won't delete)
    Total     int
}
```

**Cleanup Strategies**:
- **Merged**: Branches fully merged into main/master
- **Stale**: No commits in last N days (default: 30)
- **Orphaned**: Tracking branches with deleted remotes
- **Combined**: Union of above strategies

**Example Output**:
```
🔍 Analyzing branches for cleanup...

Merged Branches (5):
  ✓ feature/user-auth (merged 15 days ago)
  ✓ fix/login-bug (merged 7 days ago)
  ✓ feature/api-v2 (merged 21 days ago)

Stale Branches (2):
  ⏰ experiment/new-ui (45 days old)
  ⏰ feature/abandoned (90 days old)

Protected (kept):
  🔒 main, develop, release/v1.0

Delete 7 branches? [y/N]
```

### 4.2 Worktree Management

#### 4.2.1 Worktree Operations

**Interface**:
```go
type WorktreeManager interface {
    // Add adds a new worktree
    Add(ctx context.Context, repo *repository.Repository, opts AddOptions) (*Worktree, error)

    // Remove removes a worktree
    Remove(ctx context.Context, repo *repository.Repository, opts RemoveOptions) error

    // List lists all worktrees
    List(ctx context.Context, repo *repository.Repository) ([]*Worktree, error)

    // Prune removes orphaned worktree metadata
    Prune(ctx context.Context, repo *repository.Repository) error
}

type AddOptions struct {
    Path         string  // Worktree path (required)
    Branch       string  // Branch name (required)
    CreateBranch bool    // Create new branch
    Force        bool    // Overwrite existing
    Detach       bool    // Detached HEAD
}

type RemoveOptions struct {
    Path   string  // Worktree path (required)
    Force  bool    // Force removal (even with uncommitted changes)
}

type Worktree struct {
    Path       string
    Branch     string
    Ref        string
    IsMain     bool
    IsLocked   bool
    IsPrunable bool
}
```

**Worktree Path Management**:
- Default location: `~/.gzh-git/worktrees/{repo-name}/{branch-name}`
- Custom paths supported
- Validate path doesn't exist or is empty
- Clean up on removal

**Example Usage**:
```bash
# Add worktree for new feature
$ gzh-git worktree add ~/work/feature-auth feature/auth

# Add worktree with new branch
$ gzh-git worktree add ~/work/fix-bug fix/login-bug --new

# List all worktrees
$ gzh-git worktree list
/home/user/projects/myapp (main)
/home/user/work/feature-auth (feature/auth)
/home/user/work/fix-bug (fix/login-bug)

# Remove worktree
$ gzh-git worktree remove ~/work/feature-auth

# Cleanup orphaned worktrees
$ gzh-git worktree prune
```

#### 4.2.2 Worktree Safety

**Safety Checks**:
1. Verify path exists and is valid
2. Check for uncommitted changes before removal
3. Prevent removal of main worktree
4. Lock worktrees during critical operations
5. Validate branch isn't checked out in another worktree

**Error Handling**:
- `ErrWorktreeExists` - Path already exists
- `ErrWorktreeDirty` - Uncommitted changes (use --force)
- `ErrWorktreeMain` - Cannot remove main worktree
- `ErrWorktreeLocked` - Worktree is locked
- `ErrBranchInUse` - Branch checked out in another worktree

### 4.3 Parallel Workflows

#### 4.3.1 Multi-Context Development

**Use Case**: Developer needs to:
1. Work on feature A
2. Quickly switch to fix urgent bug
3. Return to feature A without losing context

**Solution with Worktrees**:
```bash
# Setup
gzh-git worktree add ~/work/feature-a feature/user-profile
gzh-git worktree add ~/work/hotfix hotfix/login-error

# Work in parallel
cd ~/work/feature-a    # Context A
# ... work on feature ...

cd ~/work/hotfix       # Context B
# ... fix bug, commit, push ...

cd ~/work/feature-a    # Back to Context A
# ... continue feature work ...
```

**Benefits**:
- No `git stash` needed
- Independent builds/tests
- No context loss
- Faster switching

#### 4.3.2 Coordination

**Shared Configuration**:
- Git config (`.git/config`) shared across worktrees
- User settings preserved
- Hooks executed per worktree

**Conflict Prevention**:
- Detect operations on same files across worktrees
- Warn on concurrent modifications
- Lock shared resources during critical operations

---

## 5. Implementation

### 5.1 File Structure

```
pkg/branch/
├── manager.go           # Core branch operations
├── manager_test.go
├── worktree.go         # Worktree management
├── worktree_test.go
├── cleanup.go          # Cleanup service
├── cleanup_test.go
├── parallel.go         # Parallel workflow helpers
├── parallel_test.go
├── types.go            # Shared types
└── errors.go           # Error definitions
```

### 5.2 Dependencies

**Internal**:
- `internal/gitcmd`: Git command execution
- `internal/parser`: Parse git output
- `pkg/repository`: Repository abstraction

**External**:
- Standard library only
- No external dependencies

### 5.3 Testing Strategy

**Unit Tests**:
- Branch creation with various options
- Branch deletion with safety checks
- Cleanup analysis accuracy
- Worktree lifecycle operations
- Error handling paths

**Integration Tests**:
- End-to-end branch workflows
- Worktree parallel operations
- Cleanup with real repository
- Performance benchmarks

**Coverage Target**: ≥85% for pkg/branch

---

## 6. CLI Commands

### 6.1 Branch Commands

```bash
# Create branch
gzh-git branch create <name> [ref]
gzh-git branch create feature/auth            # From HEAD
gzh-git branch create fix/bug v1.0.0          # From tag
gzh-git branch create feature/api --checkout  # Create and checkout

# Delete branch
gzh-git branch delete <name>
gzh-git branch delete feature/old              # Local only
gzh-git branch delete feature/old --remote     # Delete remote too
gzh-git branch delete feature/old --force      # Force delete

# List branches
gzh-git branch list
gzh-git branch list --merged                   # Only merged
gzh-git branch list --remote                   # Remote branches

# Cleanup
gzh-git branch cleanup
gzh-git branch cleanup --merged                # Merged only
gzh-git branch cleanup --stale                 # Stale only
gzh-git branch cleanup --all                   # All strategies
gzh-git branch cleanup --dry-run               # Preview only
```

### 6.2 Worktree Commands

```bash
# Add worktree
gzh-git worktree add <path> <branch>
gzh-git worktree add ~/work/fix fix/login     # Existing branch
gzh-git worktree add ~/work/feat feat/new --new  # New branch

# Remove worktree
gzh-git worktree remove <path>
gzh-git worktree remove ~/work/fix
gzh-git worktree remove ~/work/fix --force     # Force removal

# List worktrees
gzh-git worktree list
gzh-git worktree list --porcelain              # Machine-readable

# Prune orphaned worktrees
gzh-git worktree prune
gzh-git worktree prune --dry-run               # Preview only
```

---

## 7. Success Criteria

### 7.1 Functional

- ✅ Branch creation works with all options
- ✅ Branch deletion respects safety checks
- ✅ Cleanup correctly identifies branches
- ✅ Worktree add/remove works reliably
- ✅ Parallel workflows function correctly

### 7.2 Performance

- ✅ Branch operations complete within NFR targets
- ✅ Cleanup scales to 100+ branches
- ✅ Worktree operations handle 5+ concurrent worktrees

### 7.3 Quality

- ✅ Test coverage ≥85%
- ✅ All linters pass
- ✅ Zero critical bugs
- ✅ Documentation complete

---

## 8. Risks & Mitigation

| Risk | Severity | Probability | Mitigation |
|------|----------|-------------|------------|
| Data loss from accidental deletion | High | Medium | Multiple confirmation prompts, dry-run mode |
| Worktree path conflicts | Medium | Medium | Path validation, clear error messages |
| Performance with many branches | Low | Low | Efficient git commands, pagination |
| Git version compatibility | Medium | Low | Version checks, feature detection |

---

## 9. Future Enhancements

**Phase 6+ Considerations**:
- Branch protection rules configuration
- Git Flow automation
- Branch templates
- Automatic upstream tracking
- Interactive branch selection (fuzzy find)
- Branch activity metrics
- Worktree templates
- Cloud worktree sync

---

## 10. References

### 10.1 Git Documentation

- [Git Branching](https://git-scm.com/book/en/v2/Git-Branching-Branches-in-a-Nutshell)
- [Git Worktree](https://git-scm.com/docs/git-worktree)
- [Branch Management](https://git-scm.com/book/en/v2/Git-Branching-Branch-Management)

### 10.2 Related Specifications

- [00-overview.md](00-overview.md) - Project overview
- [10-commit-automation.md](10-commit-automation.md) - Phase 2 spec
- [30-history-analysis.md](30-history-analysis.md) - Phase 4 spec (pending)

### 10.3 External Tools

- [git-extras](https://github.com/tj/git-extras) - Similar branch tools
- [git-town](https://github.com/git-town/git-town) - Branch workflow automation

---

**Specification Status**: Ready for Review
**Next Steps**: Review → Approve → Implementation
**Estimated Implementation**: 1 week (Week 4)
