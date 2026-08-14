# Architecture Design Document

**Project**: gzh-cli-gitforge
**Version**: 2.1
**Last Updated**: 2026-07-16
**Status**: Active

> 상세 설계는 주제별로 [`docs/10-architecture/`](docs/10-architecture/README.md)에
> 분할되어 있습니다. 이 문서는 개요와 진입점 역할을 합니다.

______________________________________________________________________

## Document Map

| 질문                                           | 이동처                                                                                        |
| ---------------------------------------------- | --------------------------------------------------------------------------------------------- |
| 전체 구조와 패키지 구성을 알고 싶다            | [10-overview.md](docs/10-architecture/10-overview.md)                                         |
| 어떤 설계 원칙을 따르는지 알고 싶다            | [20-design-principles.md](docs/10-architecture/20-design-principles.md)                       |
| 각 컴포넌트의 역할과 책임이 궁금하다           | [30-component-design.md](docs/10-architecture/30-component-design.md)                         |
| 인터페이스/API 시그니처를 찾고 있다            | [40-interface-contracts.md](docs/10-architecture/40-interface-contracts.md)                   |
| 데이터 흐름과 에러 처리 방식이 궁금하다        | [50-data-flow-and-error-handling.md](docs/10-architecture/50-data-flow-and-error-handling.md) |
| 테스트 구조와 모킹 전략을 알고 싶다            | [60-testing-architecture.md](docs/10-architecture/60-testing-architecture.md)                 |
| 성능 특성과 보안 모델이 궁금하다               | [70-performance-and-security.md](docs/10-architecture/70-performance-and-security.md)         |
| 빌드·배포 방식을 알고 싶다                     | [80-deployment.md](docs/10-architecture/80-deployment.md)                                     |
| 왜 이렇게 설계했는지, 앞으로의 계획이 궁금하다 | [90-design-decisions.md](docs/10-architecture/90-design-decisions.md)                         |

전체 목차: [`docs/10-architecture/README.md`](docs/10-architecture/README.md)

______________________________________________________________________

## 1. Executive Summary

### 1.1 Architectural Goals

gzh-cli-gitforge adopts a **Library-First Architecture** with the following goals:

1. **Dual-Purpose Design**: Function as both standalone CLI and reusable Go library
1. **Clean Separation**: Zero coupling between library code (`pkg/`) and CLI code (`cmd/`)
1. **Maximum Reusability**: Enable easy integration into gzh-cli and other projects
1. **Interface-Driven**: All core functionality via well-defined interfaces
1. **Testability**: 100% mockable components for comprehensive testing

### 1.2 Key Architectural Decisions

| Decision                       | Rationale                                     | Trade-offs                            |
| ------------------------------ | --------------------------------------------- | ------------------------------------- |
| Library-First over CLI-First   | Enables reuse in gzh-cli; better API design   | More upfront design effort            |
| Git CLI over go-git library    | Maximum compatibility; simpler                | External dependency on Git            |
| Interfaces over concrete types | Testability; extensibility                    | More files, indirection               |
| Functional options pattern     | API extensibility without breaking changes    | More boilerplate                      |
| Context propagation            | Cancellation, timeouts, request-scoped values | Every function signature includes ctx |

______________________________________________________________________

## Appendix

### A.1 Key Files Summary

| File                           | Purpose             | Criticality |
| ------------------------------ | ------------------- | ----------- |
| `pkg/repository/interfaces.go` | Core repository API | CRITICAL    |
| `pkg/branch/manager.go`        | Branch management   | HIGH        |
| `internal/gitcmd/executor.go`  | Git command wrapper | CRITICAL    |
| `cmd/gz-git/main.go`           | CLI entry point     | MEDIUM      |

### A.2 Dependencies

```go
// go.mod
module github.com/gizzahub/gzh-cli-gitforge

go 1.25.1

require (
    github.com/fsnotify/fsnotify v1.9.0
    github.com/gizzahub/gzh-cli-core v0.0.0-20251230045225-725b628c716a
    github.com/google/go-github/v66 v66.0.0
    github.com/spf13/cobra v1.10.2
    gitlab.com/gitlab-org/api/client-go v1.46.0
    golang.org/x/oauth2 v0.34.0
    golang.org/x/sync v0.19.0
    gopkg.in/yaml.v3 v3.0.1
)
```

### A.3 References

- [Effective Go](https://golang.org/doc/effective_go.html)
- [Go Project Layout](https://github.com/golang-standards/project-layout)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [gzh-cli Architecture](https://github.com/gizzahub/gzh-cli/blob/main/ARCHITECTURE.md)

______________________________________________________________________

## Revision History

| Version | Date       | Author      | Changes                                                                         |
| ------- | ---------- | ----------- | ------------------------------------------------------------------------------- |
| 1.0     | 2025-11-27 | Claude (AI) | Initial architecture design                                                     |
| 2.0     | 2026-01-23 | Claude (AI) | Add provider, reposync, workspace packages                                      |
| 2.1     | 2026-07-16 | Claude (AI) | Split sections 2-13 into `docs/10-architecture/`; root retains overview + index |

______________________________________________________________________

**End of Document**
