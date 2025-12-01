# Documentation Structure Plan

> **Purpose**: Define the future organization of gzh-cli-git documentation
> **Status**: Planning Phase
> **Last Updated**: 2025-12-01

## Current Situation

### Existing Documentation

```
docs/
├── QUICKSTART.md              # Quick start guide
├── INSTALL.md                 # Installation instructions
├── LIBRARY.md                 # Library usage guide
├── TROUBLESHOOTING.md         # Common problems
├── COVERAGE.md                # Test coverage report
├── API_STABILITY.md           # API stability policy
├── RELEASE_NOTES_*.md         # Release notes
├── commands/
│   └── README.md              # Command reference
├── user/
│   └── guides/
│       └── faq.md             # FAQ (new)
├── llm/
│   └── CONTEXT.md             # LLM context (new)
└── phase-*.md                 # Development progress

Root Level:
├── README.md                  # Project overview
├── ARCHITECTURE.md            # Detailed architecture (1500+ lines)
├── PRD.md                     # Product requirements
├── REQUIREMENTS.md            # Technical requirements
├── CONTRIBUTING.md            # Contribution guide
└── CHANGELOG.md               # Version history
```

### Issues with Current Structure

1. **Mixed Audiences**: Human and LLM docs not clearly separated
2. **Flat Hierarchy**: Most docs at root or `docs/` level
3. **Inconsistent Naming**: Some docs user-focused, others dev-focused
4. **Navigation Difficulty**: Hard to find specific information
5. **Duplicate Content**: Information scattered across multiple files

## Proposed Structure

### Target Organization

```
docs/
├── user/                      # END-USER DOCUMENTATION
│   ├── getting-started/
│   │   ├── README.md          # Overview
│   │   ├── installation.md    # Installation guide
│   │   └── first-steps.md     # Tutorial (new)
│   ├── guides/
│   │   ├── README.md          # Guide index
│   │   ├── cli-usage.md       # CLI comprehensive guide
│   │   ├── workflows.md       # Common workflows
│   │   ├── faq.md             # FAQ (exists)
│   │   └── troubleshooting.md # Troubleshooting
│   ├── reference/
│   │   ├── commands.md        # All commands (from commands/)
│   │   ├── configuration.md   # Config file reference
│   │   └── exit-codes.md      # Exit code reference
│   └── examples/
│       ├── basic-usage.md
│       ├── feature-workflow.md
│       ├── hotfix-workflow.md
│       └── parallel-development.md
│
├── developer/                 # DEVELOPER/LIBRARY DOCUMENTATION
│   ├── library/
│   │   ├── README.md          # Library overview
│   │   ├── quickstart.md      # 5-minute library start
│   │   ├── api-reference/
│   │   │   ├── repository.md  # pkg/repository API
│   │   │   ├── operations.md  # pkg/operations API
│   │   │   ├── commit.md      # pkg/commit API (planned)
│   │   │   ├── branch.md      # pkg/branch API (planned)
│   │   │   ├── history.md     # pkg/history API (planned)
│   │   │   └── merge.md       # pkg/merge API (planned)
│   │   └── integration/
│   │       ├── basic.md
│   │       ├── gzh-cli.md
│   │       ├── error-handling.md
│   │       └── testing.md
│   ├── architecture/
│   │   ├── README.md          # High-level overview
│   │   ├── design-principles.md
│   │   ├── design-decisions.md
│   │   └── detailed-spec.md   # Full ARCHITECTURE.md content
│   ├── contributing/
│   │   ├── README.md          # How to contribute
│   │   ├── setup.md           # Dev environment setup
│   │   ├── testing.md         # Testing guide
│   │   ├── code-style.md      # Coding standards
│   │   └── pr-process.md      # Pull request process
│   └── project/
│       ├── prd.md             # Product requirements
│       ├── requirements.md    # Technical requirements
│       ├── roadmap.md         # Development roadmap
│       └── changelog.md       # Version history
│
├── llm/                       # LLM-SPECIFIC DOCUMENTATION
│   ├── CONTEXT.md             # Project context summary (exists)
│   ├── api-catalog.md         # All public APIs catalog
│   ├── codebase-map.md        # Directory/file structure guide
│   └── implementation-guide.md # How to implement features
│
└── meta/
    ├── documentation-plan.md  # This document
    ├── style-guide.md         # Documentation style guide
    └── templates/             # Document templates
        ├── feature-spec.md
        ├── api-doc.md
        └── tutorial.md
```

### Root Level Documentation

```
/
├── README.md                  # KEEP: Project overview
├── CONTRIBUTING.md            # MOVE TO: docs/developer/contributing/
├── CHANGELOG.md               # MOVE TO: docs/developer/project/
├── ARCHITECTURE.md            # MOVE TO: docs/developer/architecture/detailed-spec.md
├── PRD.md                     # MOVE TO: docs/developer/project/
├── REQUIREMENTS.md            # MOVE TO: docs/developer/project/
└── LICENSE                    # KEEP: License file
```

## Migration Strategy

### Phase 1: Create New Structure (Week 1)

1. Create new directory structure
2. Add README.md to each major section
3. Create navigation index documents

### Phase 2: Move Existing Docs (Week 1-2)

1. **User Docs**:
   - Move QUICKSTART.md → `docs/user/getting-started/README.md`
   - Move INSTALL.md → `docs/user/getting-started/installation.md`
   - Move TROUBLESHOOTING.md → `docs/user/guides/troubleshooting.md`
   - Move commands/README.md → `docs/user/reference/commands.md`

2. **Developer Docs**:
   - Move ARCHITECTURE.md → `docs/developer/architecture/detailed-spec.md`
   - Move PRD.md → `docs/developer/project/prd.md`
   - Move REQUIREMENTS.md → `docs/developer/project/requirements.md`
   - Move CONTRIBUTING.md → `docs/developer/contributing/README.md`
   - Move CHANGELOG.md → `docs/developer/project/changelog.md`

3. **Library Docs**:
   - Move LIBRARY.md → `docs/developer/library/README.md`
   - Create quickstart guide
   - Create API reference pages

### Phase 3: Create New Content (Week 2-3)

1. **User Documentation**:
   - Write workflows.md (common use cases)
   - Write example scenarios
   - Create configuration reference

2. **Developer Documentation**:
   - Write architecture overview (condensed from ARCHITECTURE.md)
   - Write design decisions document
   - Write contribution workflow guide

3. **LLM Documentation**:
   - Create API catalog
   - Create codebase map
   - Write implementation guide

### Phase 4: Update Links (Week 3)

1. Update all internal links in documentation
2. Update links in README.md
3. Update links in code comments
4. Add redirects for old paths (if needed)

### Phase 5: Cleanup (Week 3)

1. Remove old documentation files
2. Archive phase-* progress docs
3. Update .gitignore if needed

## Documentation Guidelines

### Audience Separation

**User Documentation** (`docs/user/`):
- **Target**: End users of the CLI tool
- **Focus**: How to use, practical examples, troubleshooting
- **Tone**: Friendly, tutorial-style, step-by-step
- **Format**: Short sections, lots of examples, visual aids

**Developer Documentation** (`docs/developer/`):
- **Target**: Go developers using the library, contributors
- **Focus**: API reference, architecture, contribution process
- **Tone**: Technical, precise, comprehensive
- **Format**: Detailed explanations, code examples, design rationale

**LLM Documentation** (`docs/llm/`):
- **Target**: AI assistants (Claude, GitHub Copilot, etc.)
- **Focus**: Project structure, APIs, implementation patterns
- **Tone**: Structured, token-efficient, comprehensive
- **Format**: Hierarchical, catalog-style, optimized for context windows

### File Size Limits

Per `~/.claude/CLAUDE.md` guidelines:

- **Ideal**: < 500 lines, < 10KB
- **Warning**: 500-1000 lines, 10-50KB
- **Split Required**: > 1000 lines, > 50KB

### Naming Conventions

- **Files**: `kebab-case.md` (e.g., `api-reference.md`)
- **Directories**: `kebab-case/` (e.g., `getting-started/`)
- **Exceptions**: `README.md`, `CHANGELOG.md`, `CONTRIBUTING.md`

### Content Principles

1. **DRY (Don't Repeat Yourself)**: Link instead of duplicating
2. **Progressive Disclosure**: Start simple, link to details
3. **Examples First**: Show code, then explain
4. **Searchable**: Use clear headings and keywords
5. **Up-to-date**: Mark outdated sections clearly

## Navigation Strategy

### Main Entry Points

1. **README.md** (Root):
   - Project overview
   - Quick links to main docs
   - Getting started link
   - Feature highlights

2. **docs/user/getting-started/README.md**:
   - Installation
   - First steps tutorial
   - Common use cases

3. **docs/developer/library/README.md**:
   - Library overview
   - Quick start
   - API reference index

4. **docs/llm/CONTEXT.md**:
   - Project context for AI
   - Quick reference
   - Implementation patterns

### Cross-linking Strategy

- Each document starts with breadcrumb navigation
- Related documents linked in "See Also" section
- Index pages for each major section
- Tag system for finding related content

## Success Metrics

### Measurable Goals

1. **Discoverability**: Users can find answers in < 3 clicks
2. **Completeness**: All features documented
3. **Accuracy**: < 5% documentation bugs reported
4. **Maintenance**: Docs updated within 1 week of code changes
5. **User Satisfaction**: Positive feedback on documentation

### Quality Checklist

For each document:
- [ ] Clear audience identified
- [ ] Appropriate level of detail
- [ ] Working code examples (if applicable)
- [ ] Links to related documents
- [ ] Spell-checked and grammar-checked
- [ ] Fits within size guidelines
- [ ] Up-to-date with current version

## Timeline

| Week | Tasks | Deliverables |
|------|-------|--------------|
| 1 | Create structure, move existing docs | New directory structure, moved files |
| 2 | Create new user content | Workflows, examples |
| 3 | Create new developer content | Architecture overview, API refs |
| 4 | Update links, cleanup | Complete migration |

## Open Questions

1. Should we use a documentation generator (e.g., MkDocs, Docusaurus)?
2. Do we need versioned documentation?
3. Should we create video tutorials?
4. Do we want interactive examples (e.g., runnable code snippets)?

## References

- Current README: [README.md](../README.md)
- CLAUDE.md Guidelines: `~/.claude/CLAUDE.md`
- File Size Limits: `~/.claude/ctx/FILE_SIZE_LIMITS.md`
- LLM Writing Guide: `~/.claude/ctx/LLM_FRIENDLY_WRITING.md`

---

**Next Steps**: Get approval for structure, begin Phase 1 migration
