# Context Documentation - gzh-cli-git

This directory contains detailed context documentation extracted from CLAUDE.md for LLM optimization.

## Purpose

Keep CLAUDE.md under 300 lines while maintaining comprehensive guidance through linked context documents.

## Files

| File | Purpose | When to Read |
|------|---------|--------------|
| [common-tasks.md](common-tasks.md) | Adding commands, testing git operations | Daily development |
| [security-guide.md](security-guide.md) | Input sanitization, safe execution (CRITICAL) | ALWAYS - before any git command code |

## Quick Access

**New to the project?** Start here:
1. Read CLAUDE.md (quick overview)
2. Read cmd/AGENTS_COMMON.md (project conventions)
3. **Read security-guide.md (CRITICAL - prevent command injection)**
4. Read common-tasks.md (see how to work)

**Adding git commands?**
- **MUST READ**: security-guide.md for input sanitization
- Check common-tasks.md for workflows

**Security is critical** for git operations:
- Never use shell execution
- Always sanitize user inputs
- Follow security checklist before committing
