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
| 07 | [llm-output-nondeterministic-map-order](issue/07-llm-output-nondeterministic-map-order.md) | P3 | **blocked**: core 정렬 수정 완료. 릴리스 → `go.mod` bump 전까지 `sortLLMSummaryBlock` 제거 불가 (`GOWORK: off` CI) |

### Recently closed (2026-08-07 residual pass)

| # | 태스크 | 결과 |
|---|--------|------|
| 08 | conflict-guard residual ExitCode audit | `status` → `runGit`; existence probes documented intentional |
| 11 | status consumer + fixture gaps | ParseStatus C/unknown-worktree; switch/status fixtures; testutil fatals |
| 16 | goheader | COMPANY=Gizzahub; linter disabled pending header-add |
| 18 | golangci v1/v2 | PATH has v2.12.2; install target skips when v2 present |
