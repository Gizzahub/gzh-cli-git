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
| 18 | [golangci-v1-binary-breaks-v2-config-lint-gate](issue/18-golangci-v1-binary-breaks-v2-config-lint-gate.md) | **P1** | v1 golangci-lint 바이너리가 v2 설정을 거부해 lint 게이트가 통째로 내려가 있음 |
| 08 | [conflict-guard-fail-open-on-git-failure](issue/08-conflict-guard-fail-open-on-git-failure.md) | P2 | **Scope 1·2·3·4 해소**. 잔여: `pkg/repository` 전역의 `executor.Run`+`ExitCode`-only 지점 선별. 10의 Finding 2·3이 같은 계열 |
| 11 | [status-consumer-and-fixture-test-gaps](issue/11-status-consumer-and-fixture-test-gaps.md) | P2 | status 소비자 7곳 무테스트 + 06에서 삭제된 케이스 미복원 + 픽스처가 실패를 삼킴 |
| 16 | [goheader-rule-rejects-every-file](issue/16-goheader-rule-rejects-every-file.md) | P3 | `.golangci.yml`이 `Archmagece`를 기대하는데 저장소 198개 파일 중 0개가 일치. `max-same-issues: 5`가 전수 위반을 5건으로 보여 lint 건수가 실행마다 흔들린다. **미결정**(저작권자 표기) |
| 07 | [llm-output-nondeterministic-map-order](issue/07-llm-output-nondeterministic-map-order.md) | P3 | `gzh-cli-core` 쪽 **수정 완료**(정렬 방출 + 100회 결정성 테스트). 릴리스 → `go.mod` bump 전까지 `sortLLMSummaryBlock` 제거 불가 — CI는 `GOWORK: off`로 pinned core를 쓴다 |
