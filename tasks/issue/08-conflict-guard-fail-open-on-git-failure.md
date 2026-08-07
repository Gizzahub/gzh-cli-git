# ISSUE: `checkRepositoryState`가 git 실패를 "충돌 없음"으로 읽어 push/pull 가드가 열린다

- status: done (2026-08-07 — residual ExitCode-only audit closed)
- priority: P2 (2026-08-05 재평가. 같은 날 내린 **P3 강등은 오판이었고 철회한다** —
  근거였던 "잔여는 표시 경로뿐"이 사실이 아니었다. 상세는 "갱신 #2")
- category: repository
- created_at: 2026-08-05T18:30:00+09:00
- affects: v0.7.0
- spawned_from: `06-porcelain-parsers-outside-diff-commit.md` (착수 전 조사 중 발견)

## Background

태스크 06은 `checkRepositoryState`의 **파싱** 결함(디렉터리 축약·C-quoting·충돌 판정 중복)을
다룬다. 조사 중 같은 블록에서 파싱과 무관한 별개 결함이 나왔다: **git 명령이 실패해도 에러가
나지 않고 "충돌 없는 깨끗한 리포"로 판정된다.**

심각도 계열이 다르므로 분리한다. 06은 표시 경로의 정확도(P3), 이건 `push`/`pull`의
**의사결정 가드가 fail-open**(P2)이다.

## Findings

### 실패 신호가 삼켜지는 지점 — `pkg/repository/bulk.go:1813-1836`

```go
statusResult, err := c.executor.Run(ctx, repoPath, "status", "--porcelain")
if err != nil {
    return nil, fmt.Errorf("failed to get repository status: %w", err)
}

if statusResult.ExitCode == 0 && statusResult.Stdout != "" {
    // ... HasConflicts / ConflictedFiles / UncommittedFiles 채움
}

return state, nil     // ExitCode != 0 → 위 블록을 통째로 건너뛰고 zero-value state 반환
```

`Executor.Run`(`internal/gitcmd/executor.go:110-172`)은 git의 실패를 `Result.ExitCode`로만
알리고 `error`에는 nil을 넣는다(인자 sanitize 실패만 error). 따라서 `err != nil` 검사는
**프로세스 기동 실패만** 잡고, git이 exit 128로 죽은 경우는 통과한다. 그 결과:

- `state.HasConflicts` = false
- `state.ConflictedFiles` = nil
- 반환 error = nil

즉 **"확인 불가"가 "이상 없음"으로 승격된다.**

이건 같은 패키지가 이미 인지하고 고친 결함 계열이다 — `runGit`
(`pkg/repository/changeset.go:224-241`)의 주석이 그대로 이 상황을 서술한다:

> `Executor.Run` reports a failed git command through `Result.ExitCode` and returns a nil
> error, so checking only err turns "git failed" into "git found nothing": a broken status
> reads as a clean repository and **a broken conflict probe reads as a conflict-free one.**

태스크 01이 `runGit`을 도입하며 diff/commit 경로는 이 계약으로 옮겼으나,
`checkRepositoryState`는 이전 형태로 남았다.

### 영향받는 호출 경로

| 호출 지점 | 앞선 `GetStatus` | 결과 |
|---|:---:|---|
| `bulk.go:2012` `processPushRepository` | **없음** (`GetInfo`만) | **가드 우회** — `"cannot push: repository has unresolved conflicts"`(`:2022`)를 통과해 push 진행 |
| `bulk.go:1668` 풀 실패 후 상태 재확인 | **없음** | `HasConflicts=false` → `abortRebaseIfNeeded`(`:1672`) 복구 스킵, 리포가 리베이스 중간 상태로 잔류. 에러도 "Pull failed"로 오분류 |
| `bulk.go:1545` `processPullRepository` | 있음(`:1535`) | 마스킹됨 — `RunOutput`은 exit≠0에 error를 내므로 `GetStatus`가 먼저 실패 |
| `bulk_switch.go:183` | 있음(`:162`) | 마스킹됨 (동일 이유) |
| `bulk.go:2442` `processStatusRepository` | 있음(`:2432`) | 마스킹됨 |

**마스킹된 3곳은 우연이다.** `GetStatus`가 앞에 있고 그게 `RunOutput`(exit≠0 → error)을
쓰기 때문일 뿐, `checkRepositoryState` 자체의 계약이 아니다. 호출 순서가 바뀌면 되살아난다.

### 재현 — git status만 실패하고 GetInfo는 통과하는 상태

```console
$ printf 'GARBAGE' > .git/index
$ git status --porcelain          ; echo "exit=$?"
fatal: .git/index: index file smaller than expected
exit=128
$ git rev-parse --abbrev-ref HEAD ; echo "exit=$?"    # GetInfo 경로는 살아있음
master
exit=0
$ git remote -v                   ; echo "exit=$?"
exit=0
```

잘린 인덱스는 디스크 풀이나 git 쓰기 중단으로 생기는데, **그건 미완료 merge가 함께 남아 있을
개연성이 높은 상황**이다. 즉 가드가 가장 필요한 순간에 열린다.

같은 계열의 fail-open이 `pkg/reposync/diagnostic_executor.go:166-168`에도 있다:

```go
if err != nil {
    logger.Warn("failed to check working tree", ...)
    health.WorkTreeStatus = WorkTreeClean // Assume clean on error
}
```

여기는 표시 경로라 심각도는 낮으나 같은 안티패턴이므로 함께 정리 대상.

## Scope

1. `checkRepositoryState`가 `c.executor.Run` 대신 `runGit`을 쓰도록 전환 —
   exit≠0을 에러로 승격. 호출자 5곳은 이미 `err != nil` 분기에서 `StatusError`를 세팅하므로
   추가 처리 불필요.
2. `if ExitCode == 0 && Stdout != ""` 가드 제거 (빈 stdout = clean은 파싱 결과로 자연히 나옴).
3. `diagnostic_executor.go:168`의 `WorkTreeClean` fallback을 `WorkTreeUnknown` 계열로 강등하거나,
   최소한 health 상태를 warning으로 표시. (별도 판단 필요 — 표시 경로라 fail-fast가 과할 수 있음)
4. 회귀 테스트: git status가 exit≠0을 내는 픽스처에서 `checkRepositoryState`가 **에러를 반환**하고,
   `processPushRepository`가 `StatusError`로 끝나는지 확인.

## Acceptance Criteria

- [x] `checkRepositoryState`가 git exit≠0에 대해 non-nil error를 반환
- [x] 손상된 인덱스 픽스처에서 `gz-git push`가 성공 경로로 진행하지 않음
- [x] 풀 실패 후 상태 재확인(현 `bulk.go:1724`)이 상태 확인 실패를 "충돌 없음"으로 오인하지 않음
- [x] `pkg/repository`에 `c.executor.Run(...)` 직접 호출 후 `ExitCode`만 검사하는 잔존 지점 없음
      (2026-08-07: 답변 불가 계열 `status` → `runGit`; 존재 탐침·표시 메타데이터는 의도적 유지 — 아래 갱신 #3)
- [x] `diagnostic_executor.go`의 "assume clean on error"에 대한 판단 기록 (수정 또는 유지 근거)

---

## 2026-08-05 갱신 — Scope 1·2·4는 태스크 06에서 해소됨

06이 `checkRepositoryState`를 재작성하면서 `c.executor.Run` → `runGit` 전환을 함께 수행했다.
06 파일의 예고("06에서 `runGit` 전환을 하게 되면 08의 Scope 1이 자연히 해소되므로 08을 닫기
전에 실제 diff로 확인할 것")대로, 실제 코드로 확인한 결과는 다음과 같다.

### 해소 (실코드 확인)

- **Scope 1** — `bulk.go:1866` `checkRepositoryState`는 이제
  `c.runGit(ctx, repoPath, "status", "--porcelain", "-z", "-uall")`을 호출하고
  `err != nil`에 `fmt.Errorf("failed to get repository status: %w", err)`로 종료한다.
  `runGit`은 exit≠0을 error로 승격하므로 "확인 불가"가 "이상 없음"으로 새지 않는다.
- **Scope 2** — `if statusResult.ExitCode == 0 && statusResult.Stdout != ""` 가드가 사라졌다.
  빈 stdout은 레코드 0건 → 빈 `Status`로 자연히 clean이 된다.
- **Scope 4 전반** — `TestCheckRepositoryStateFailsOnBrokenGit`
  (`pkg/repository/porcelain_test.go`)이 `.git/index`를 손상시킨 픽스처에서
  `err != nil && state == nil`을 검증한다. 이 계약이 되돌아가면 테스트가 깨진다.
- **호출자 3곳 확인**:
  - `bulk.go:2077` `processPushRepository` — `err != nil`에 `StatusError` + 조기 반환.
    가드를 통과해 push로 진행하는 경로가 없다.
  - `bulk.go:1724` 풀 실패 후 재확인 — `stateErr != nil`이면 "Couldn't check state" 분기로
    간다. 이전에는 fail-open 때문에 `stateErr`가 항상 nil이라 `HasConflicts=false`
    분기로 흘렀다. **다만 이 분기에서도 `abortRebaseIfNeeded`는 여전히 호출되지 않는다** —
    이건 의도적으로 남긴다. 상태를 읽지 못하는 상황에서 rebase abort는 사용자의 작업 트리를
    임의로 되돌리는 파괴적 동작이고, 지금은 최소한 `StatusError`로 표면화된다.
  - `bulk_switch.go:189` — 동일하게 `err != nil` 분기 보유.

### 잔여 범위 (08이 계속 열려 있는 이유)

1. **`diagnostic_executor.go:166-168`의 `WorkTreeClean // Assume clean on error`** — 미수정.
   06은 같은 블록의 카운트 계열만 고쳤다. 판단 기록이 아직 없다.
2. **`pkg/repository` 전역의 `executor.Run` + `ExitCode`-only 잔존 지점** — 조사 결과
   생각보다 넓다. 특히 같은 fail-open **계열**인 곳:

   | 위치 | 코드 | 결과 |
   |---|---|---|
   | `bulk_stash.go:275` | `statusResult, _ := ...Run(..., "status", "--porcelain")` 후 `ExitCode == 0 && Stdout == ""` | git 실패 시 "변경 있음"으로 읽혀 stash를 시도 — 방향은 fail-safe지만 근거는 우연 |
   | `bulk_tag.go:265,300`, `bulk_cleanup.go:333,409` | `_, _ :=` 로 error 자체를 버리고 `ExitCode == 0`만 검사 | 태그/브랜치 존재 판정이 "확인 불가 = 없음"으로 붕괴 |
   | `bulk.go:1676,1767,1777` | `rev-parse HEAD` / `rev-list --count` | 실패 시 빈 문자열 → "변경 없음"으로 판정 |

   여기서 필요한 건 일괄 치환이 아니라 **지점별 판단**이다. 위 표의 `//nolint:errcheck`
   주석들은 전부 "ExitCode check below handles both"라고 주장하지만, `rev-parse --verify`
   처럼 exit≠0이 정상 답변("없음")인 경우와 `status`처럼 exit≠0이 **답변 불가**인 경우가
   섞여 있다. 전자는 현행 유지가 맞고 후자만 `runGit`으로 옮겨야 한다.

### 우선순위 재평가 (철회됨 — 아래 "갱신 #2" 참조)

> ~~`push` 가드가 닫혔으므로 **P2 → P3**. 남은 것은 표시 경로(`diagnostic_executor`)와
> 비가역성이 낮은 판정들(태그/브랜치 존재 확인, HEAD 비교)이다.~~
>
> **오판. 같은 날 세션 리뷰에서 철회했다.** "표시 경로라 심각도가 낮다"는 전제가
> 틀렸다 — 아래 참조.

---

## 2026-08-05 갱신 #2 — Scope 3 해소, 강등 철회

### 강등이 오판이었던 이유

P3 강등의 근거는 "잔여는 표시 경로(`diagnostic_executor`)뿐"이었다. 그러나 그
"표시 경로"는 `gz-git status`의 **판정 그 자체**를 만든다. `WorkTreeClean` fallback은
`classifyHealth` → `HealthHealthy` → `generateRecommendation` → JSON `summary.healthy`
까지 그대로 전파된다. 즉 이 fallback은 화면 문구를 고르는 게 아니라 **"이 리포는
조치가 필요 없다"는 최종 판정을 증거 없이 만들어낸다.**

실측 (`.git/index`를 손상시킨 픽스처, `git status` exit=128 / `rev-parse` exit=0):

| | v0.7.0 | 수정 후 |
|---|---|---|
| `status --skip-fetch` | `✓ All 1 repositories are healthy` | `✗ 1 error` |
| exit code | **0** | **2** |
| `summary` | `healthy:1` | `error:1` |
| `recommendation` | `No action needed, repository is up-to-date` | `Working tree state could not be read...` |
| `error` 필드 | 없음 | `...exited 128: fatal: .git/index: index file smaller than expected` |

**인덱스가 깨진 리포가 "정상"으로 보고되고 exit 0으로 끝난다.** 표시 경로라서 낮은
심각도가 아니라, 표시 경로이기 때문에 사용자가 이걸 근거로 다음 행동을 정한다.

추가로, 이 fallback은 **상류 파서 결함의 증폭기**로도 작동했다. 태스크 06이
`GetStatus`를 `-z` 파서로 옮기며 들어온 rename 페어링 결함(` R` 미처리)은 그 자체로는
시끄러운 에러(`unknown index status code: h`)였는데, 이 fallback이 그걸 조용한
"healthy"로 바꿨다. 파서 결함 1건 + fail-open 1건 = **오답이 무증상이 되는 조합**.
어느 한쪽만 있었다면 즉시 드러났을 것이다.

### Scope 3 해소 (실코드)

- `WorkTreeUnknown` 상태 신설 (`diagnostic.go`) — `WorkTreeClean`과 명시적으로 구분한다.
  "물었는데 답을 못 받았다"와 "답이 '변경 없음'이다"는 다른 사실이다.
  `WorkTreeStatus`의 zero value(`""`)는 여전히 "검사 안 함"(`CheckWorkTree=false`)을 뜻한다.
- `diagnostic_executor.go` — `WorkTreeClean` fallback 제거, `WorkTreeUnknown` +
  `health.Error` 세팅. git의 원본 에러 문자열이 JSON `error` 필드까지 전파된다.
- `classifyHealth` — `WorkTreeUnknown` → `HealthError`. **내용 기반 판정(conflict/dirty/
  divergence)보다 앞에** 둔다. 그 판정들은 전부 우리가 갖고 있지 않은 status 출력에
  근거하므로, 이 상태에서 "충돌 없음"·"깨끗함"은 증거의 부재이지 부재의 증거가 아니다.
  `HealthWarning`이 아니라 `HealthError`인 이유: 인덱스 손상·중단된 git 쓰기는
  **미완료 merge가 함께 남아 있을 개연성이 높은** 상태다.
- 렌더러 2곳 (`pkg/tui/formatter.go`, `pkg/reposynccli/status_command.go`) —
  두 switch 모두 `default`가 없어 새 상태가 **아무 문구도 없이** 렌더링됐다.
  `state-unreadable` / `UNREADABLE` 케이스 추가.
- 회귀 테스트 — `TestCheckHealthDoesNotCallUnreadableTreeHealthy`
  (`pkg/reposync/diagnostic_worktree_test.go`, 태그 없음 = 기본 실행).
  **뮤테이션 검증 완료**: fallback을 되돌리면 이 테스트가
  `HealthStatus = "healthy"` / `Recommendation = "No action needed..."`로 실패한다.
  `classifyHealth` 테이블에도 2케이스 추가.

### 잔여 범위 (08이 계속 열려 있는 이유 — 갱신)

Scope 1·2·4는 06에서, Scope 3은 이번에 해소됐다. 남은 것은 아래 하나다.

**`pkg/repository` 전역의 `executor.Run` + `ExitCode`-only 지점 선별.** 위 "잔여 범위"
표(태그/브랜치 존재 판정, `rev-parse`/`rev-list`)는 그대로 유효하다. 필요한 건 일괄
치환이 아니라 **지점별 판단**이다: `rev-parse --verify`처럼 exit≠0이 정상 답변("없음")인
경우와 `status`처럼 exit≠0이 **답변 불가**인 경우가 섞여 있다. 전자는 현행 유지가 맞고
후자만 `runGit`으로 옮겨야 한다. 이 잔여가 P2인 이유는 태그/브랜치 "없음" 오판이
`bulk_tag`/`bulk_cleanup`의 생성·삭제 분기를 바꾸기 때문이고, P1이 아닌 이유는
비가역 경로(push/커밋)가 이미 닫혔기 때문이다.

## 2026-08-07 갱신 #3 — residual ExitCode-only 지점별 판단 완료

답변 불가(`status`)와 존재 탐침(`rev-parse --verify`)을 분리했다.

| 위치 | 판단 | 조치 |
|---|---|---|
| `bulk_stash.go` `processStashSave` `status --porcelain` | exit≠0 = 답변 불가 | **`runGit`으로 전환**, StatusError |
| `bulk_tag.go` `rev-parse --verify refs/tags/...` | exit≠0 = 태그 없음 (정상 답) | 유지 + 의도 주석 |
| `bulk_tag.go` `tag -l` | 표시 메타; 실패→0개 | 유지 + 의도 주석 |
| `bulk_cleanup.go` `branch --list` | 표시 카운트 | 유지 + 의도 주석 |
| `bulk_cleanup.go` `detectBaseBranch` `rev-parse --verify` | exit≠0 = 후보 없음 | 유지 + 의도 주석 |
| `bulk_stash.go` `stash list` | 표시 카운트 | 유지 + 의도 주석 |
| `bulk.go` `rev-parse HEAD` / `rev-list --count` (pull 전후) | 실패 시 비교 스킵/fallback 1 | 유지 — push 가드와 무관, 성공 후 통계 |

AC "ExitCode만 검사하는 잔존 지점 없음"은 **fail-open 계열(답변 불가→이상 없음)** 이 없다는 뜻으로 해석한다.
존재 탐침에서 exit≠0을 "없음"으로 읽는 것은 git의 계약 자체이므로  residual이 아니다.

## References

- 관련 태스크: `06-porcelain-parsers-outside-diff-commit.md` (같은 함수, 파싱 계열 결함)
- 계약 근거: `pkg/repository/changeset.go:224-241` (`runGit` 주석이 이 결함을 명시적으로 서술)
- 원본 감사: workflow `wf_6c7e7604-0aa` (해당 감사는 이 건을 잡지 못함 — diff/commit 스코프 한정)
