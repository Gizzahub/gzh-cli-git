# Tasks History — gzh-cli-gitforge

> 완료 기록·감사 이력 보관. 인덱스 아님 — 열린 항목은 [README.md](README.md) 참조.

______________________________________________________________________

## 완료 (2026-08-14 — quality-debt follow-up)

| #   | 태스크                                                                                     | 우선순위 | 요지                                                                                                                |
| --- | ------------------------------------------------------------------------------------------ | -------- | ------------------------------------------------------------------------------------------------------------------- |
| 07  | [llm-output-nondeterministic-map-order](issue/07-llm-output-nondeterministic-map-order.md) | P3       | published core `84a0f3d`를 `go.mod`/`go.sum`에 반영해 `GOWORK=off` 단독 CI도 정렬 formatter를 사용 ✅               |
| 21  | [golangci-exclusion-paths-unanchored](issue/21-golangci-exclusion-paths-unanchored.md)     | P1       | exclusion 경로 교정·254건 기준선 측정·lint-zero 게이트 복구; 범위별 suppression 부채는 문서에 지연 항목으로 명시 ✅ |

검증·범위 메모: handoff의 untracked-only 실제 CLI 경로와 artifact guard는
`tests/e2e/handoff_test.go`에 추가했다. 린트 0건은 게이트 복구를 뜻하며, 모든 기존
lint 지적을 근본 해결했다는 의미는 아니다. 남은 항목은 [issue 21의 지연된 린트 부채](issue/21-golangci-exclusion-paths-unanchored.md#%EC%A7%80%EC%97%B0%EB%90%9C-%EB%A6%B0%ED%8A%B8-%EB%B6%80%EC%B1%84)에 링크한다.

______________________________________________________________________

## 완료 (2026-08-07)

| #   | 태스크                                                                                     | 우선순위 | 요지                                                                                                                                          |
| --- | ------------------------------------------------------------------------------------------ | -------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| 07  | [llm-output-nondeterministic-map-order](issue/07-llm-output-nondeterministic-map-order.md) | P3       | core `WriteLLM` 맵 정렬 후 `sortLLMSummaryBlock` 제거; 로컬 go.work 기준. 단독 모듈 CI는 core pseudo-version publish 후 `go.mod` bump 필요 ✅ |

______________________________________________________________________

## 완료 (2026-08-05)

| #   | 태스크                                                                                                             | 우선순위 | 요지                                                                                                                                                                                                                                                   |
| --- | ------------------------------------------------------------------------------------------------------------------ | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 01  | [changeset-unify-diff-commit-scope](issue/01-changeset-unify-diff-commit-scope.md)                                 | **P0**   | `diff`/`commit`이 변경집합 정의를 공유하지 않음 — 공용 `collectChangeSet` 도입 ✅                                                                                                                                                                      |
| 02  | [commit-merge-conflict-guard](issue/02-commit-merge-conflict-guard.md)                                             | **P0**   | 미해결 merge conflict를 그대로 커밋 (유일한 비가역 손상) ✅                                                                                                                                                                                            |
| 03  | [untracked-read-loop-security-and-oom](issue/03-untracked-read-loop-security-and-oom.md)                           | **P0**   | `--include-untracked`의 `os.ReadFile` 루프 — 정보유출·OOM·무음 누락 ✅                                                                                                                                                                                 |
| 04  | [commit-stats-accuracy-numstat](issue/04-commit-stats-accuracy-numstat.md)                                         | P2       | `additions`/`deletions` 부정확, 실패가 exit code에 안 드러남 ✅                                                                                                                                                                                        |
| 05  | [diff-output-untracked-visibility](issue/05-diff-output-untracked-visibility.md)                                   | P2       | default/compact 포맷에 untracked 신호 전무 ✅                                                                                                                                                                                                          |
| 06  | [porcelain-parsers-outside-diff-commit](issue/06-porcelain-parsers-outside-diff-commit.md)                         | P3       | 동일 porcelain 결함이 `bulk.go`·`client.go`의 상태/헬스 경로에 잔존 — 패키지 단일 파서(`porcelain.go`)로 통합, `internal/parser.ParseStatus` 삭제 ✅ <br>⚠️ **완료 후 세션 리뷰에서 신규 파서에 P0 유입 확인** — 수정 완료, 아래 "P0 유입과 수정" 참조 |
| 10  | [porcelain-parsers-outside-pkg-repository](issue/10-porcelain-parsers-outside-pkg-repository.md)                   | P2       | `pkg/repository` 밖 재파싱 6곳 — `internal/porcelain` 단일 파서로 통합, `AA`/`DD` 충돌 누락 수정, 읽기 실패를 "정상"으로 렌더하던 3경로 정정 ✅ (2026-08-06)                                                                                           |
| 14  | [worktree-get-does-not-resolve-symlinks](issue/14-worktree-get-does-not-resolve-symlinks.md)                       | P2       | `Get`의 `filepath.Abs`가 심볼릭 링크를 안 풀어 macOS `/var` 하위 워크트리를 못 찾음 — `samePath`로 교체 ✅ (2026-08-06)                                                                                                                                |
| 13  | [branch-manager-cleanup-fail-open-on-git-failure](issue/13-branch-manager-cleanup-fail-open-on-git-failure.md)     | P2       | `manager.go`·`cleanup.go` 10곳이 실패한 git을 성공으로 읽음 — 패키지 `runGit` 도입. `Execute`가 삭제 실패를 버리던 것을 `ExecuteResult`로 반환, `✓ Deleted N`이 후보 수 대신 삭제 수를 출력하고 실패 시 exit 2 ✅ (2026-08-06)                         |
| 15  | [cleanup-gone-flag-is-a-no-op](issue/15-cleanup-gone-flag-is-a-no-op.md)                                           | P2       | 단일 리포 `--gone`이 무동작 — 독립 결함 3개. "고아"의 정의가 벌크 경로와 달랐던 것이 본질(로컬 `[gone]` vs 원격추적 ref). `findGoneBranches`로 통일, `IncludeGone` 추가, CLI 배선 ✅ (2026-08-06)                                                      |
| 12  | [internal-parser-is-dead-code](issue/12-internal-parser-is-dead-code.md)                                           | P3       | 임포터 0건 패키지 `internal/parser` 삭제(소스 8.9KB + 테스트 21KB). `internal/` 하위라 공개 API 변경 아님. 살아있는 `parseAheadBehind` **3개** 사본은 통합 대상이 아니라 별건으로 보존 ✅ (2026-08-06)                                                 |
| 09  | [porcelain-parser-silently-skips-malformed-records](issue/09-porcelain-parser-silently-skips-malformed-records.md) | P2       | 06 통합 시 계약이 조용히 바뀜 — 구 파서는 형식 오류에 에러, 신규는 skip ✅                                                                                                                                                                             |
| 17  | [execute-skips-protected-screen-when-exclude-empty](issue/17-execute-skips-protected-screen-when-exclude-empty.md) | P3       | `cleanup.Execute`의 보호 브랜치 검사가 `len(opts.Exclude) > 0`일 때만 실행되던 것 — 항상 스크리닝 ✅                                                                                                                                                   |

**검증**: `go build`·`go vet`·`gofumpt`·전체 테스트 통과.
01~05는 `golangci-lint`(신규 결함 0)까지 통과. 06 시점에는 설치본이 go1.25로 빌드되어 있고
모듈이 go1.26을 타깃해 `golangci-lint`가 기동 자체를 못 한다(환경 문제, 변경과 무관).

실 `gzh-cli-gitforge` 작업 트리에서 diff(6+22)/commit--dry-run(28)/`git add -A`(28) 3자 일치 확인.
06은 추가로 `info`/`status` 경로를 git ground truth와 대조 (아래 표).

### 의존 관계 (실행 완료)

```
01 (change-set 통일) ──┬── 04 (stats 정확도)     ✅
                       ├── 05 (출력 가시성)      ✅
                       └── 06 (상태/헬스 경로)   ✅ ─┬─ 08 Scope 1·2·4 동반 해소
                                                     └─ P0 유입 → 세션 리뷰에서 적발·수정
                                                        (그 과정에 08 Scope 3도 해소)
02 (conflict guard)  ── 독립                    ✅
03 (untracked 읽기)  ── 독립                    ✅
```

**착수 순서**(02 → 03 → 01 → 04 → 05 → 06)대로 완료.

### 06 실측 대조 (`uncommitted` + `untracked`)

| 픽스처                                                | git 실제 | v0.7.0 | 수정 후    |
| ----------------------------------------------------- | -------- | ------ | ---------- |
| 이 리포 (`info`)                                      | 18 + 3   | 21 + 3 | **18 + 3** |
| 워크트리 삭제 2 + untracked 디렉터리(파일 2) (`info`) | 2 + 2    | 3 + 1  | **2 + 2**  |
| 같은 픽스처 (`status --skip-fetch`, 헬스 경로)        | 2 + 2    | 1 + 1  | **2 + 2**  |

### 06 공개 API 변경 (CHANGELOG 기재됨)

- `RepositoryFetchResult`/`PullResult`/`PushResult`/`StatusResult`의 `UncommittedFiles`
  **deprecate** → `TrackedChangedFiles` + `StagedFiles`/`UnstagedFiles` 신설.
  값은 정정되고 **JSON 키는 불변**(`uncommitted_files` 유지).
- `Status`에 `StagedCount`/`UnstagedCount`/`TrackedChangedCount` 신설.
- `internal/parser.ParseStatus` **삭제** (프로덕션 호출자 0건).

______________________________________________________________________

### 2026-08-05 — 06 착수 전 조사에서 추가 확인 (전부 처리됨)

- **08 (신규)**: `Executor.Run`이 exit≠0에 nil error를 돌려주는데 `checkRepositoryState`가
  `err`만 검사한다. `processPushRepository`는 앞단에 `GetStatus`가 없어 마스킹되지 않는다.
  잘린 `.git/index`로 재현 확인 (status exit=128, `rev-parse` exit=0).
  → **06의 `runGit` 전환으로 해소**, `TestCheckRepositoryStateFailsOnBrokenGit`이 고정.
  P2였던 이유(push 가드 fail-open)가 사라져 ~~**P3로 강등**~~, 잔여 범위만 남기고 열어 둠.
  → **이 강등은 오판이었고 같은 날 철회했다.** "잔여는 표시 경로뿐"이라는 근거가 사실이
  아니었다 — `diagnostic_executor.go`의 fail-open이 남아 있었고, 그것이 `gz-git status`를
  "healthy, exit 0"으로 만드는 증폭기였다. **P2로 환원.**
- **06에 흡수**: `RunOutput`의 `strings.TrimSpace`(`executor.go:211`)가 `parseStatus`의
  컬럼 오프셋 파싱과 충돌해 **첫 레코드**를 오독한다. v0.7.0 실측 재현
  (워크트리 전용 삭제 2건 → `uncommitted_files: 1`, 기대 0). → 해소.
- **`UncommittedFiles`는 표시 전용 확인** → 06의 P3 유지. 실제 더티 가드는 전부
  `status.IsClean`(불리언, 축약에 둔감)을 쓴다. `repositoryState.IsDirty`는 리더 0건.
  → 필드 deprecate + 세분화로 처리, `IsDirty`는 삭제.
- **`internal/parser.ParseStatus`는 프로덕션 호출자 0건** → 삭제로 종결.
- **06 진행 중 추가 발견**: 구 `parseStatusCode`는 `T`(typechange)를 `default`로 흘려
  파싱 전체를 실패시켰다. 신규 파서에서 정상 분류 + 회귀 테스트.

### 2026-08-05 (저녁) — 07 수정

- **07 수정 완료** (`gzh-cli-core`, 미커밋). 헬퍼 제거만 릴리스에 묶여 있다 —
  로컬은 `go.work`로 수정본을, CI는 `GOWORK: off`로 pinned 구버전을 쓴다. 실측 확인.
- **`internal/parser`는 패키지 전체가 죽은 코드**. 06이 지운 `ParseStatus` 외 7함수도
  호출자 0건이고, `pkg/doctor`는 `parseAheadBehind` 사본을 따로 갖고 있다. → 10에 병합.
- 09·10·11은 병행 실행된 `/quality:review:session`이 생성. 10에 **검증 결과 1건 정정**을
  덧붙였다(아래 태스크 문서 참조).

### 2026-08-05 (밤) — 06이 유입시킨 P0 적발과 수정

세션 리뷰가 **06 자체에 P0를 유입시켰음**을 찾아냈다. 06은 "porcelain 파서 통합"이었고,
통합된 신규 파서가 rename 페어링을 잘못 구현했다.

#### 인과 사슬 (전부 실측 확인)

```
porcelain.go:59  rename 소스 레코드를 Code[0]로만 페어링
  ↓  git은 ` R dst\0src\0` 도 낸다 (mv a b && git add -N b)
소스 경로 handler.go 가 스트림에 남아 상태 라인으로 재해석
  ↓  Code="ha"
applyStatusCode → "unknown index status code: h"  ← 여기까진 시끄러운 실패
  ↓
diagnostic_executor.go:168  GetStatus 에러를 WorkTreeClean 으로 삼킴  ← 증폭기
  ↓
classifyHealth → HealthHealthy
  ↓
"No action needed, repository is up-to-date", exit 0   ← 조용한 오답
```

**두 결함이 겹쳐야 성립한다.** 하나만 있었으면 시끄러운 에러로 끝났다.

#### 바이너리 실측 (`tmp/bin/gz-git`, v0.7.0 대조)

| 픽스처                                  | v0.7.0                                     | 수정 후                             |
| --------------------------------------- | ------------------------------------------ | ----------------------------------- |
| `mv` + `add -N` (`status --skip-fetch`) | `✓ healthy`                                | **`⚠ [1 modified]`**                |
| 같은 픽스처 (`info`)                    | `Status: error`                            | **`Status: dirty (1 uncommitted)`** |
| `.git/index` 파손 (`status`)            | `✓ All 1 repositories are healthy`, exit 0 | **`✗ 1 error`, exit 2**             |

세 번째 줄은 **06 이전부터 있던 v0.7.0 fail-open**이다. `diagnostic_executor.go` 수정의
부수 효과로 함께 닫혔다.

#### 수정 내역

| 파일                                                        | 변경                                                                                                                                         |
| ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `pkg/repository/porcelain.go`                               | `isRenameOrCopyCode` — X/Y 어느 쪽이든 `R`/`C`면 페어링. worktree측 `R`/`C`를 `RenamedFiles`에, ` A`(intent-to-add)를 `ModifiedFiles`에 분류 |
| `pkg/reposync/diagnostic.go`                                | `WorkTreeUnknown` 신설 — `WorkTreeClean`과 구분                                                                                              |
| `pkg/reposync/diagnostic_executor.go`                       | fail-open 제거. 읽기 실패 → `WorkTreeUnknown` + `HealthError` + 에러 보존                                                                    |
| `pkg/tui/formatter.go`, `pkg/reposynccli/status_command.go` | 신규 상태 렌더링 (`default` 없는 switch라 안 넣으면 무음 누락)                                                                               |
| `pkg/repository/bulk.go`                                    | `checkRepositoryState`: `-uall` → `-uno` (`ConflictFiles`만 읽으므로 효과 0인 전체 워크)                                                     |
| `docs/.claude-context/common-tasks.md`                      | 06이 지운 안티패턴을 그대로 가르치고 있던 절 교체                                                                                            |

#### 뮤테이션 검증 (테스트가 실제로 붙잡는지)

| 되돌린 수정          | 결과                                                                                                        |
| -------------------- | ----------------------------------------------------------------------------------------------------------- |
| 페어링 → `Code[0]`만 | `TestParseStatus`·`TestGetStatusPairsWorktreeSideRename`·`TestCheckHealthSeesWorktreeSideRename` FAIL       |
| fail-open 복원       | `TestCheckHealthDoesNotCallUnreadableTreeHealthy`가 `clean`/`healthy`/`No action needed`로 P0 재현하며 FAIL |

둘 다 되돌림 완료. 06에 있던 `TestCheckRepositoryStateExpandsUntrackedDirectories`는
**아무것도 검증하지 않는 테스트**여서 `...SeparatesDirtyFromConflicted`로 교체했다.

#### 왜 06에서 안 걸렸나 (11로 추적)

- 파서 테이블 테스트에 ` R` 케이스가 없었다 — 있는 줄 알았던 건 `R ` 였다.
- `pkg/repository` × `pkg/reposync` 교차 테스트가 `//go:build integration` 뒤에 있어
  기본 `go test ./...`에서 **한 번도 실행되지 않았다**. 신규
  `diagnostic_worktree_test.go`는 의도적으로 태그를 붙이지 않았다.

### 2026-08-06 — 10 완료 (12·13 분리)

최초 판정 "재파싱 6곳 중 2곳만 결함" → 재조사 결과 **5곳이 결함**. 오판의 축은 하나다:
각 지점이 불리언 하나만 본다는 관찰은 옳았고, 놓친 것은 **그 불리언을 읽어내는 방법**이었다.
커밋 `174c0ab`(`internal/porcelain` 신설) → `b0caf10`(reposync) → `7d1fc41`(doctor) →
`7eecc6c`(branch/worktree + sync 프리뷰). 상세·결정 근거는 태스크 문서 참조.

범위가 달라 분리한 잔여: **12**(죽은 `internal/parser` 삭제), **13**(`pkg/branch`
`manager.go`·`cleanup.go` 잔여 fail-open 10곳), **14**(`worktreeManager.Get` 심볼릭
링크 미해석 — 실측 재현됨). 13 조사 중 **15**(`cleanup branch --gone` 무동작) 파생.

### 2026-08-06 — 14·13·15 완료

**14** (`4679fdc`): `samePath`가 철자 비교 후 `EvalSymlinks`로 떨어진다. 철자 비교를
먼저 두는 것은 최적화가 아니라 **디렉터리가 이미 삭제된 워크트리를 매칭할 수 있는 유일한
경로**다 — git은 그런 워크트리도 나열하고, 제거는 호출자가 실제로 하는 일이다.

**13**: `gz-git cleanup branch`가 일어나지 않은 삭제를 보고하던 결함 3중첩을 해소.
git이 전부 거부해도 `✓ Deleted N` + exit 0이 나왔다. 패키지 `runGit` 도입(10곳),
`Execute`는 `(*ExecuteResult, error)`로 바뀌어 `Deleted`/`Failed`를 돌려준다 —
집계 에러를 쓰지 않은 이유는 **부분 성공이 전체 실패로 읽히기 때문**(태스크 D1).
`Exists`는 exit code가 곧 답이라 헬퍼에서 제외하고 회귀 테스트를 붙였다.

**15**: 단일 리포 `--gone` 무동작. 미결로 남겼던 `--gone` × `--remote` 관계는 **고를
필요가 없었다** — 벌크 경로(`bulk_cleanup.go:462`)가 이미 (c)를 구현하고 있었다.
착수하고 나서야 결함이 접두어 버그 하나가 아니라 **"고아"의 정의가 두 경로에서 서로
달랐던 것**임이 드러났다: 벌크는 git이 `[gone]`으로 표시한 **로컬** 브랜치, 단일은
"등록되지 않은 remote를 가리키는 **원격추적** ref". 이름만 같고 다른 개념이다.
후자를 버리고 `findGoneBranches`로 통일했으며, 여기 딸린 독립 결함 둘(`AnalyzeOptions`에
`IncludeGone` 부재, 단일 경로 CLI가 플래그를 `Analyze`에 미전달)도 함께 고쳤다.
셋 중 어느 하나만 고쳐도 `--gone`은 여전히 무동작이었다.

### 2026-08-06 — 12 완료

`internal/parser` 패키지 삭제. 임포터 0건은 착수 직전에 다시 확인했다 — 태스크 작성과
착수 사이에 새 호출자가 생겼을 수 있으므로 문서의 과거 측정을 믿지 않는다.

착수 중 Findings의 사실 하나가 틀린 것을 확인했다. "`pkg/doctor`가 사본을 따로 갖고
있다"고 단수로 적었으나 실제로는 **셋**이다(`pkg/repository/client.go:468`,
`pkg/branch/manager.go:441`, `pkg/doctor/repo_checks.go:469`). 셋 다 살아 있고
호출자가 있어 이 태스크의 대상이 아니다 — 통합은 정본 시그니처 선택이 걸린 별개 판단이다.

`parser/`만 지우면 구조표가 여전히 틀리므로 `internal/` 하위를 실측(`gitcmd`·
`porcelain`·`config`·`testutil`)에 맞췄다. **`docs/specs/` 3건은 정정하지 못했다** —
전역 규칙이 `specs/`를 AI 수정 금지로 지정한다. 저장소 소유자 조치 필요(태스크 D3에
파일·행 명시). CHANGELOG의 4건은 이력이므로 의도적으로 남겼다.

잔여: **08**·**11**은 미착수. **16**·**17**은 신규(P3), 16은 미결정 포함.

### 2026-08-07 — 17·19 완료

**17**: `cleanup.Execute`가 `Exclude`가 비면 보호 브랜치 스크리닝을 건너뛰던 것 수정 —
항상 보호 브랜치를 걸러낸다.

**19**: `tasks/README.md`가 10KB 가이드라인을 2배 초과 → 완료·감사 기록을 이 파일
(`HISTORY.md`)으로 분리. README에는 구조·Open Issues 인덱스만 유지.

______________________________________________________________________

## 감사 이력

### 2026-08-05 — `diff`/`commit` 변경집합 일관성 감사

발단: `gz-git diff`(4파일)와 `gz-git commit --dry-run`(7파일)의 보고 범위가 달라, LLM 에이전트에 넘길 커밋 메시지 근거에서 ADR 2건과 태스크 파일 10건이 누락됨.

다중 에이전트 감사(17 agents, 4 lens × 적대적 검증) 결과 **12건 확인, 0건 기각**. 이 중 7건은 수동 재현으로 독립 확인.

#### Finding → 태스크 매핑

| Finding ID                                           | 위치                                       | 태스크 | 수동재현 |
| ---------------------------------------------------- | ------------------------------------------ | ------ | :------: |
| (원 보고) 스코프 불일치                              | `bulk_diff.go:284` vs `bulk_commit.go:316` | 01     |    ✅    |
| `diff-omits-staged-changes`                          | `bulk_diff.go:309`, `:332`                 | 01     |          |
| `commit-preview-undercounts-untracked-dirs`          | `bulk_commit.go:301`                       | 01     |    ✅    |
| `porcelain-quoted-paths-never-unquoted`              | `bulk_diff.go:268`, `bulk_commit.go:320`   | 01     |    ✅    |
| `commit-commits-merge-conflict-markers`              | `bulk_commit.go:333`, `:368`               | 02     |    ✅    |
| `untracked-symlink-dereference-leak`                 | `bulk_diff.go:349`                         | 03     |    ✅    |
| `include-untracked-noop-on-directories`              | `bulk_diff.go:349`                         | 03     |    ✅    |
| `include-untracked-silently-drops-files`             | `bulk_diff.go:349`                         | 03     |          |
| `untracked-read-unbounded-memory`                    | `bulk_diff.go:349`                         | 03     |          |
| `commit-stats-sum-not-head-delta`                    | `bulk_commit.go:336`, `:342`               | 04     |          |
| `commit-additions-double-count-staged-plus-unstaged` | `bulk_commit.go:342`                       | 04     |          |
| `commit-untracked-lines-never-counted`               | `bulk_commit.go:336-346`                   | 04     |          |
| `parse-diff-stats-filename-poisoning`                | `bulk_commit.go:478`                       | 04     |          |
| default/compact 포맷 untracked 미표시                | `diff.go:194`, `:263`                      | 05     |    ✅    |

> 12건이 5개 태스크로 묶인 이유: 여러 finding이 **동일한 수정 지점**을 공유한다.
> `include-untracked-*` 4건은 `bulk_diff.go:343-367` 한 블록, `commit-stats-*` 4건은
> `bulk_commit.go:336-346`의 합산 로직 하나에서 파생된다. 태스크는 "고칠 단위"로 나눴다.

#### 근본 원인

`diff`와 `commit`이 `git status --porcelain`을 **각자 재파싱**하며, `??` 라인 처리와 라인 수 산출 근거가 서로 다르다. 그런데 최종 실행자 `executeCommit`은 `git add -A`를 돌린다. 실제 변경집합의 정의는 **HEAD → worktree(untracked 포함)** 하나인데, **이를 계산하는 코드가 저장소에 없다** — `git diff HEAD`는 한 번도 호출되지 않는다.

`internal/parser/status.go:119`에 이미 porcelain 분류 로직(conflict `U` 포함)이 존재하나 두 bulk 경로 모두 이를 쓰지 않고 재구현했다. 이 중복이 문제의 표면적 징후다.

______________________________________________________________________

## v0.7.0 사용자 우회법 (수정 전까지)

핵심: **스캔 전에 스테이징해서 변경집합 계산을 git에게 넘긴다.**

```bash
# 1) conflict 사전 차단 (gz-git은 검사하지 않음)
git -C <repo> ls-files -u | head -1        # 비어있지 않으면 제외
test -e <repo>/.git/MERGE_HEAD && exit 1

# 2) 스테이징 후 --staged 로 증거 수집 (executeCommit이 어차피 하는 일과 동일)
git -C <repo> add -A
gz-git diff <dir> --staged --max-size 500

# 3) 커밋 후 대조
git -C <repo> show --name-only HEAD
```

- `--include-untracked`는 **쓰지 말 것** — 태스크 03의 4개 결함 전부가 이 플래그 경로에서만 발생한다.
- `files_changed`/`additions`/`deletions`는 신뢰하지 말고 `git diff --cached --numstat`로 직접 구할 것.
- 커밋 실패 시에도 exit code가 0이므로, `--format json`의 `summary.error`와 `repositories[].status`를 반드시 파싱할 것.
