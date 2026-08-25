# Tasks — gzh-cli-gitforge

> Last Updated: 2026-08-25

## 구조

```
tasks/
├── issue/       # 확인된 결함 / 후속 설계 필요 항목 (이 저장소가 소유)
├── HISTORY.md   # 완료·감사 기록
└── README.md    # 열린 항목 인덱스 (이 파일)
```

착수 대기·진행 중 **작업 큐는 여기에 두지 않는다.** devbox 루트가 소유한다 —
아래 [Task ownership](#task-ownership) 참조.

규약: **1파일 = 1작업**, `NN-kebab-case-title.md`, 인덱스 파일은 `README.md`/`INDEX.md`만.
(`gzh-cli/tasks/` 규약 준용)

완료·감사 기록: [HISTORY.md](HISTORY.md)

______________________________________________________________________

## Open Issues (후속)

| #   | 이슈                                           | 요약                                                                                                                                      |
| --- | ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| 28  | goreleaser-brews-deprecated-and-ci-pins-latest | 저장소 코드·CI 수정은 완료. 빈 `gizzahub/homebrew-tap` bootstrap, 최초 Cask 게시·macOS 설치, stable release workflow의 실제 검증이 남았다 |

메인테이너는 다음 stable 버전을 v0.8.0으로 결정했고 `VERSION`과 릴리스 노트를 준비했다.
실제 태그 발행은 아직 하지 않는다. 빈 `homebrew-tap` 기본 브랜치, 최소 권한
`HOMEBREW_TAP_TOKEN`, 최초 Cask 게시·감사·macOS 설치가 모두 확인된 뒤에만
`v0.8.0` 태그를 발행한다.

### Recently closed (2026-08-25)

| #   | 태스크                                            | 결과                                                                                                                               |
| --- | ------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| 26  | defaults-scan-depth-is-a-dead-config-key          | 모든 bulk scan에 계층형 기본 깊이를 적용하고 명시 플래그 우선·설정 확장자·반복 실행 계약을 테스트로 고정 (`7fa61c5`·`341ce6d`)     |
| 27  | task-branch-upstream-points-at-integration-branch | integration upstream 별도 진단, 안전한 task ref 처방, audit code 추가와 slash remote 정규화로 해결 (`b8ad882`·`9a42ae4`·`f1c9881`) |
| 25  | no-declarative-exclusion-for-bulk-write-commands  | `defaults.scan.exclude` 추가(`2dcaf8e`)와 빈 항목 제거(`33cc679`)로 해결. 저장소 단위 `readOnly`는 채택하지 않음                   |

### Recently closed (2026-08-24)

| #   | 태스크                                        | 결과                                                                                                                                                                                        |
| --- | --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 22  | changelog-exceeds-doc-size-gate               | 과거 릴리스 6개 라인을 `docs/changelog/`로 분할(1320줄/63KB → 559줄/38KB), 71커밋 백로그를 주제별로 서술, 항목 밀도 규칙과 `<!-- size-limit: 700 -->` 예산 명시                             |
| 24  | make-changelog-overwrites-handwritten-log     | `.make/dev.mk`의 `changelog:` 타깃 제거(후보 A). 설정도 도구도 없어 실행된 적 없는 파괴 경로였고, 기계 목록은 `.goreleaser.yaml`이 담당                                                     |
| 23  | golangci-cache-reports-removed-worktree-paths | `e7f631f`/`052325e`가 per-run `GOLANGCI_LINT_CACHE`와 저장소 밖 진단 차단을 이미 구현. 관측된 유령 237건은 설치 바이너리가 낡아서였다 — 게이트 수정은 `make install` 뒤에야 판정에 반영된다 |

### Recently closed (2026-08-14 quality-debt follow-up)

| #   | 태스크                                 | 결과                                                                                                                                                                                                                                                                        |
| --- | -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 07  | llm-output-nondeterministic-map-order  | drop `sortLLMSummaryBlock`; consume published sorted core in `go.mod` (`51aadf0`), including `GOWORK=off` CI                                                                                                                                                                |
| 21  | golangci-exclusion-paths-unanchored    | anchor `vendor`/`tmp`, remove unnecessary `.git` exclusion, measure 254 baseline, restore lint-zero gate; [deferred lint debt](issue/21-golangci-exclusion-paths-unanchored.md#%EC%A7%80%EC%97%B0%EB%90%9C-%EB%A6%B0%ED%8A%B8-%EB%B6%80%EC%B1%84) remains explicitly scoped |
| 08  | conflict-guard residual ExitCode audit | `status` → `runGit`; existence probes documented intentional                                                                                                                                                                                                                |
| 11  | status consumer + fixture gaps         | ParseStatus C/unknown-worktree; switch/status fixtures; testutil fatals                                                                                                                                                                                                     |
| 16  | goheader                               | COMPANY=Gizzahub; linter disabled pending header-add                                                                                                                                                                                                                        |
| 18  | golangci v1/v2                         | PATH has v2.12.2; install target skips when v2 present                                                                                                                                                                                                                      |

### Task ownership

Work items for this library are tracked at the **devbox root**:

→ `gzh-cli-devbox/tasks/` (e.g. todo/40-gz-git-forge-event-stream-product.md)

Local `tasks/issue/20-…` is a **pointer only**.
