# Tasks — gzh-cli-gitforge

> Last Updated: 2026-08-07

## 구조

```
tasks/
├── issue/    # 확인된 결함 / 후속 설계 필요 항목
├── todo/     # 착수 대기 (범위 확정됨)
├── doing/    # 진행 중
└── done/     # 완료
```

규약: **1파일 = 1작업**, `NN-kebab-case-title.md`, 인덱스 파일은 `README.md`/`INDEX.md`만.
(`gzh-cli/tasks/` 규약 준용)

완료·감사 기록: [HISTORY.md](HISTORY.md)

---

## Open Issues (후속)

| # | 태스크 | 우선순위 | 요지 |
|---|--------|---------|------|
| — | (none tracked as open in this residual pass) | — | 추가 이슈는 `issue/` 디렉터리 참조 |

### Recently closed (2026-08-07 residual pass)

| # | 태스크 | 결과 |
|---|--------|------|
| 07 | llm-output-nondeterministic-map-order | drop `sortLLMSummaryBlock`; local go.work uses sorted core; publish core + go.mod bump for `GOWORK=off` CI |
| 08 | conflict-guard residual ExitCode audit | `status` → `runGit`; existence probes documented intentional |
| 11 | status consumer + fixture gaps | ParseStatus C/unknown-worktree; switch/status fixtures; testutil fatals |
| 16 | goheader | COMPANY=Gizzahub; linter disabled pending header-add |
| 18 | golangci v1/v2 | PATH has v2.12.2; install target skips when v2 present |

### Follow-up (2026-08-07)

| # | Task | Priority | Notes |
|---|------|----------|-------|
| 20 | [forge-event-stream-product-surface](issue/20-forge-event-stream-product-surface.md) | P3 | Transferred residual from gzh-cli issue 26 (remote ListEvents) — product surface on gz-git when prioritized |

