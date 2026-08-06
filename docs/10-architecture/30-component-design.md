# 4. Component Design

> gzh-cli-gitforge 아키텍처 문서 · [인덱스](README.md) · [ARCHITECTURE.md](../../ARCHITECTURE.md)

### 4.1 Directory Structure

```
gzh-cli-gitforge/
├── pkg/                          # PUBLIC API
│   ├── repository/               # Core repo operations
│   │   ├── interfaces.go         # Repository, Client interfaces
│   │   ├── client.go             # Client implementation
│   │   ├── types.go              # Repository, Info, Status types
│   │   └── options.go            # Functional options
│   ├── branch/                   # Branch management
│   │   ├── interfaces.go         # BranchManager interface
│   │   ├── manager.go            # Manager implementation
│   │   ├── worktree.go           # Worktree operations
│   │   ├── workflow.go           # Parallel workflows
│   │   └── types.go              # Branch, Worktree, etc.
│   ├── history/                  # Git history analysis
│   │   ├── interfaces.go         # Analyzer interface
│   │   ├── analyzer.go           # Analyzer implementation
│   │   ├── stats.go              # Statistics calculation
│   │   ├── contributors.go       # Contributor analysis
│   │   └── types.go              # Commit, Statistics, etc.
│   ├── merge/                    # Advanced merge/rebase
│   │   ├── interfaces.go         # MergeManager interface
│   │   ├── manager.go            # Manager implementation
│   │   ├── conflict.go           # Conflict detection
│   │   ├── strategies.go         # Resolution strategies
│   │   └── types.go              # ConflictReport, etc.
│   ├── config/                   # Configuration
│   │   ├── config.go             # Config struct
│   │   └── validation.go         # Config validation
│   ├── provider/                 # Forge providers (github/gitlab/gitea)
│   │   ├── github.go             # GitHub API client
│   │   ├── gitlab.go             # GitLab API client
│   │   └── gitea.go              # Gitea API client
│   ├── reposync/                 # Repo sync planner/executor
│   │   ├── planner.go            # Sync planning logic
│   │   └── executor.go           # Sync execution
│   ├── reposynccli/              # Sync CLI commands
│   ├── workspacecli/             # Workspace CLI commands
│   ├── scanner/                  # Local git repo scanner
│   ├── cliutil/                  # CLI utilities
│   └── templates/                # Configuration templates
│
├── internal/                     # INTERNAL (not exposed)
│   ├── gitcmd/                   # Git command execution
│   │   ├── executor.go           # Command executor
│   │   ├── sanitize.go           # Input sanitization
│   │   └── errors.go             # Error types
│   └── validation/               # Input validation
│       ├── validator.go          # Validation logic
│       └── patterns.go           # Regex patterns
│
├── cmd/                          # CLI APPLICATION
│   └── gz-git/                  # Binary: gz-git
│       ├── main.go               # Entry point
│       ├── root.go               # Root command
│       └── internal/             # CLI-specific (not reusable)
│           ├── cli/              # Cobra commands
│           │   ├── branch/       # Branch commands
│           │   ├── history/      # History commands
│           │   └── merge/        # Merge commands
│           ├── output/           # Output formatting
│           │   ├── table.go      # Table renderer
│           │   ├── json.go       # JSON formatter
│           │   └── formatter.go  # Common interface
│           └── ui/               # User interface
│               ├── progress.go   # Progress bars
│               └── prompt.go     # User prompts
│
├── examples/                     # Library usage examples
│   ├── basic/                    # Basic usage
│   ├── branch/                   # Branch features
│   └── gzh_cli_integration/      # gzh-cli integration
│
├── test/                         # Integration & E2E tests
│   ├── integration/              # Integration tests
│   └── e2e/                      # End-to-end tests
│
└── configs/                      # Default configurations
    └── templates/                # Configuration templates
```

### 4.2 Component Diagram

```
┌─────────────────────────────────────────────────────────┐
│                     pkg/repository                       │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Client (interface)                               │   │
│  │  - Open(ctx, path) (*Repository, error)         │   │
│  │  - Clone(ctx, opts) (*Repository, error)        │   │
│  │  - GetStatus(ctx, repo) (*Status, error)        │   │
│  └──────────────────┬───────────────────────────────┘   │
│                     │ implements                         │
│  ┌──────────────────▼───────────────────────────────┐   │
│  │ client (struct)                                   │   │
│  │  - executor: *gitcmd.Executor                    │   │
│  │  - logger: Logger                                │   │
│  └───────────────────────────────────────────────────┘   │
└──────────────────────┬────────────────────────────────┘
                       │ uses
        ┌──────────────▼────────────────┐
        │   internal/gitcmd/Executor    │
        │  - Run(ctx, dir, args)        │
        └───────────────┬────────────────┘
                        │ executes
            ┌───────────▼───────────┐
            │   Git CLI (external)   │
            └────────────────────────┘
```
