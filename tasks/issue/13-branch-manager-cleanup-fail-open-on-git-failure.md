# ISSUE: `pkg/branch`의 manager·cleanup이 실패한 git을 성공으로 읽는다

- status: todo
- priority: P2
- category: branch
- created_at: 2026-08-06T10:10:00+09:00
- affects: v0.7.0+
- spawned_from: `10-porcelain-parsers-outside-pkg-repository.md` Follow-ups

## Background

태스크 10 Scope 4가 `pkg/branch/worktree.go`의 같은 결함 5곳을 고쳤다(`7eecc6c`).
그때 `manager.go`·`cleanup.go`는 **지점마다 개별 판단이 필요해** 커밋 범위 밖으로
미뤘다. 이 태스크가 그 잔여다.

함정은 하나다:

> `gitcmd.Executor.Run`은 **실패한 git에도 nil error를 돌려준다.**
> 실패는 `Result.ExitCode`에 있고, non-nil error는 프로세스를 *띄우지 못한* 경우뿐이다.

따라서 `if _, err := Run(...); err != nil` 은 **모든 git 실패를 성공으로 받아들인다.**

## Findings

### A. `Run` + `ExitCode` 미검사 11곳

| 위치 | 명령 | 실패 시 현재 동작 | 심각도 |
|---|---|---|---|
| `manager.go:108` | `branch <name> <ref>` | `Create`가 **nil 반환 = 생성 성공**. 브랜치는 없다 | 높음 |
| `manager.go:114` | `checkout <name>` | `Create(Checkout:true)`가 성공 반환. **HEAD는 그대로**라 이후 커밋이 엉뚱한 브랜치에 쌓인다 | 높음 |
| `manager.go:177` | `branch -d/-D <name>` | `Delete`가 성공 반환. `-d`가 미머지 브랜치를 거부한 것이 **"삭제됨"으로 보고**된다 | 높음 |
| `manager.go:190` | `push <remote> --delete` | 원격 삭제 실패가 성공으로 보고 | 중간 |
| `manager.go:225` | `branch -vv` | `List`가 빈 슬라이스 + nil. **"브랜치가 없는 리포지터리"** — `worktree.go`에서 고친 것과 같은 형태 | 중간 |
| `manager.go:288` | `rev-parse --abbrev-ref HEAD` | `Current`가 빈 이름으로 `Get("")` 호출 | 중간 |
| `manager.go:90` | `rev-parse --verify <ref>` | `ErrInvalidRef` 가드가 **작동하지 않는다**. 뒤의 `:108`이 어차피 실패하므로 최종 결과는 실패지만 에러 종류가 틀린다 | 낮음 |
| `cleanup.go:193` | `branch --merged <base>` | `isBranchMerged` → `false` = "미머지". 삭제 방향으로는 안전하나 정리 대상이 조용히 사라진다 | 낮음 |
| `cleanup.go:214` | `log -1 --format=%ct` | 빈 stdout → 뒤의 `Sscanf`가 실패해 **우연히** 에러가 난다. 의도된 방어가 아니다 | 낮음 |
| `cleanup.go:248` | `remote` | `isBranchOrphaned` → `true` = **"고아"**. 유일하게 삭제 방향으로 열린 지점 (실제 도달 가능성은 태스크 15 참조) | 중간 |

`manager.go:315`(`Exists`)는 **올바르다** — `result.ExitCode == 0`을 의도적으로
불리언으로 쓴다. 고치지 말 것.

### B. 삭제 실패가 사용자에게 도달하는 경로가 없다

`cleanup.go:165`

```go
if err := c.branchManager.Delete(ctx, repo, deleteOpts); err != nil {
    // Log error but continue with other branches
    // In a real implementation, we'd use a logger here
    continue
}
```

에러를 버린다. 그리고 `cmd/gz-git/cmd/cleanup_branch.go:168`

```go
fmt.Printf("\n✓ Deleted %d branch(es)\n", report.CountBranches())
```

`CountBranches()`는 **삭제된 수가 아니라 보고서의 후보 수**다
(`cleanup.go:282`). git 실패가 하나도 없어도 `Execute`의 `Exclude` 필터가
`toDelete`를 줄이면 이미 틀린다.

A와 B가 겹치면 **실패한 삭제를 사용자가 알 수 있는 경로가 전혀 없다** —
`Delete`가 실패를 성공으로 바꾸고(A), 남은 진짜 에러는 `Execute`가 버리고(B),
최종 메시지는 후보 수를 센다(B).

## Scope

1. `worktree.go`의 `run` 헬퍼(`7eecc6c`)와 같은 패턴을 `manager`·`cleanupService`에
   도입한다. 헬퍼 위치는 두 타입이 공유하도록 파일 단위가 아닌 패키지 단위로 둔다.
   ```go
   func run(ctx context.Context, ex *gitcmd.Executor, dir string, args ...string) (*gitcmd.Result, error)
   ```
2. 표 A의 10곳을 헬퍼로 교체한다. **`manager.go:315`는 제외.**
3. `manager.go:90`의 `ErrInvalidRef` 가드가 실제로 발화하도록 고친다 —
   `rev-parse --verify`의 exit≠0이 곧 "그런 ref 없음"이다.
4. `cleanup.Execute`가 삭제 실패를 버리지 않게 한다. 최소한 실패 목록을 반환값에
   담아 CLI가 셀 수 있게 한다. 한 브랜치 실패로 전체를 중단하지 않는 현행 정책은
   유지 — 바꿀 것은 "계속한다"가 아니라 "조용히 계속한다"다.
5. `✓ Deleted N branch(es)`가 **실제 삭제 수**를 출력하게 한다. 실패가 있으면
   그 수도 함께 보고한다.
6. CHANGELOG 기재.

## Acceptance Criteria

- [ ] git이 실패하는 상황에서 `Create`·`Create(Checkout:true)`·`Delete`·`List`·
      `Current`가 각각 non-nil error를 돌려주는 테스트
- [ ] `Create(Checkout:true)`가 성공을 반환했다면 HEAD가 실제로 옮겨갔음을 확인하는 테스트
- [ ] 존재하지 않는 ref로 `Create` 시 `errors.Is(err, ErrInvalidRef)`
- [ ] `Exists`가 여전히 "없는 브랜치 → `false, nil`"을 지킨다 (2번의 회귀 방지)
- [ ] 삭제 실패가 하나라도 있으면 CLI 출력에 드러난다 (숫자 또는 목록)
- [ ] `✓ Deleted N` 의 N이 후보 수가 아니라 삭제 수임을 확인하는 테스트
- [ ] `make quality` 통과
- [ ] CHANGELOG 기재

## References

- 선행: `10-...md` Scope 4 / 커밋 `7eecc6c` (`worktree.go`의 동일 결함 5곳 + `run` 헬퍼)
- 같은 계열: `08-conflict-guard-fail-open-on-git-failure.md` (`pkg/repository` 쪽 잔여)
- 파생: `15-cleanup-gone-flag-is-a-no-op.md` (`isBranchOrphaned` 도달성)
- 전역 규칙: `error-visibility` (오류 숨김/무시/삼킴 금지, fail-fast)
