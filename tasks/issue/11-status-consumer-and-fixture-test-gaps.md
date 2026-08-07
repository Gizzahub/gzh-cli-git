# ISSUE: status 소비자 7곳 무테스트 + 06에서 삭제된 케이스 미복원 + 픽스처가 실패를 삼킨다

- status: done (2026-08-07)
- priority: P2
- category: repository, testing
- created_at: 2026-08-05T21:50:00+09:00
- affects: v0.7.0
- spawned_from: `/quality:review:session` (06 완료 직후 세션 리뷰)

## Background

태스크 06은 porcelain 파서를 통합하면서 **파서 자체의** 테이블 테스트는 남겼지만,
그 파서의 출력을 소비하는 함수들은 여전히 테스트가 없다. P0(rename 페어링)가
`gz-git status`를 "healthy, 조치 불필요, exit 0"로 만들 때까지 살아남은 이유가
정확히 이 공백이다 — 파서 단위 테스트는 ` R` 케이스를 갖고 있지 않았고,
소비자 테스트는 아예 없었다.

## Findings

### 1. 06에서 삭제됐고 아직 복원되지 않은 테이블 케이스

`internal/parser/status.go`가 지워질 때 함께 사라진 3건:

| 케이스 | 현재 상태 |
|---|---|
| `C ` (copied) | **미복원** — `applyStatusCode`의 `case 'C'` 분기가 무테스트 |
| unknown worktree status code | **미복원** — `default: return fmt.Errorf(...)` 경로가 무테스트 |
| short record | **태스크 09 소관** — 09가 "에러여야 한다"로 계약을 바꾸므로 그쪽에서 복원 |

앞의 2건은 이번 세션에서 `applyStatusCode`의 worktree 스위치를 **다시 쓴** 직후라
특히 중요하다. `case 'R', 'C'`는 새로 추가된 코드인데 `C` 경로에 테스트가 없다.

### 2. 소비자 7곳 무테스트

| 위치 | 함수 | 무엇을 하는가 |
|---|---|---|
| `pkg/repository/bulk.go:1416` | `populateFetchDirtyStatus` | fetch 결과에 dirty 필드 채움 |
| `pkg/repository/bulk.go:1817` | `populatePullDirtyStatus` | pull 결과에 dirty 필드 채움 |
| `pkg/repository/bulk.go:2359` | `populateDirtyStatus` | push 결과에 dirty 필드 채움 |
| `pkg/repository/bulk.go:2462` | `processStatusRepository` | `gz-git status` 한 저장소 처리 |
| `pkg/repository/update.go:303` | (update 경로) | `GetStatus` 결과로 갱신 여부 판단 |
| `pkg/repository/update.go:400` | (update 경로) | 동일 |
| `pkg/repository/bulk_switch.go:162` | (switch 경로) | dirty면 브랜치 전환 건너뜀 |

세 `populate*DirtyStatus`는 서로 거의 같은 코드다 — 하나가 틀리면 셋 다 틀린다.
`bulk_switch.go:162`은 **사용자 데이터 보호 판정**이다: 여기서 dirty를 놓치면
작업 중인 변경 위에 브랜치를 갈아끼운다.

### 3. reposync 회귀 케이스 2건 (문서에만 있고 테스트 없음)

이번 세션에 `pkg/reposync/diagnostic_worktree_test.go`가 생기면서 일부는 덮였다.
남은 건 `diagnostic_integration_test.go`가 `//go:build integration` 태그로 묶여
**기본 `go test ./...`에서 한 번도 실행되지 않는다**는 사실 자체다. P0가 여기를
빠져나간 경로이므로, 태그 정책을 재검토해야 한다 — 네트워크가 필요 없는 케이스는
태그를 떼거나 별도 파일로 분리.

### 4. 픽스처가 실패를 삼킨다 (`internal/testutil/testutil.go`)

```go
cmd = exec.Command("git", "commit", "-m", "Initial commit")
cmd.Dir = dir
if err := cmd.Run(); err != nil {
    t.Logf("git commit warning (non-fatal in test setup): %v", err)
}
```

- **`t.Logf`로 넘어간다.** `git commit`이 실패해도 함수는 정상 반환한다.
  `TempGitRepoWithCommit`이라는 이름과 달리 커밋 없는 저장소를 돌려주고,
  이걸 쓰는 테스트는 "HEAD 없음" 상태를 검증하게 된다 — 조용히 다른 것을 테스트한다.
- **`commit.gpgsign`을 끄지 않는다.** 사용자 전역 설정에 서명이 켜져 있으면
  위 `git commit`이 실패하고, 위 이유로 조용히 통과한다. 개발자 머신 의존.
- `init.defaultBranch`도 미설정 — 브랜치 이름을 하드코딩하는 테스트가 있으면 깨진다.
  (이번 세션의 `mergeConflictFixture`는 `rev-parse --abbrev-ref HEAD`로 읽어서 회피)

## Scope

1. `TestParseStatus`에 `C` copied·unknown worktree code 케이스 추가.
2. 소비자 7곳에 대해 실 git 픽스처 기반 테스트 추가. 최소한
   `bulk_switch.go:162`(데이터 보호)과 `processStatusRepository`(사용자 노출 경로)는 필수.
3. `populate*DirtyStatus` 3중복은 테스트 추가 전에 하나로 합칠지 판단
   (합치면 테스트 1개로 3곳이 덮인다 — 단, 별도 태스크로 뺄 것).
4. `internal/testutil`: `t.Logf` → `t.Fatalf`, `commit.gpgsign=false`·
   `init.defaultBranch` 명시.
5. `diagnostic_integration_test.go`의 `integration` 태그 재검토 — 로컬 git만
   쓰는 케이스는 기본 실행 대상으로.

## Acceptance Criteria

- [x] `C` copied 케이스가 `TestParseStatus`에 존재하고 `RenamedFiles`를 검증
      (`copied file` + `worktree-side copy`; index-side C now fills RenamedFiles)
- [x] unknown worktree code가 non-nil error를 반환함을 검증하는 케이스 존재
      (`unknown worktree status code` — ` X weird.txt`)
- [x] `bulk_switch.go` dirty 판정에 실 픽스처 테스트 존재 (dirty면 전환 안 함)
      (`TestProcessSwitchRepositorySkipsDirty`)
- [x] `processStatusRepository`에 실 픽스처 테스트 존재
      (`TestProcessStatusRepositoryReportsDirty` / `...ReportsClean`)
- [x] `TempGitRepoWithCommit`이 커밋 실패 시 `t.Fatalf`
- [x] `testutil` 픽스처가 `commit.gpgsign=false` 설정 (+ `init.defaultBranch=main`)
- [x] `integration` 태그 없이 실행되는 reposync 회귀 테스트 유지/확대
      (`diagnostic_worktree_test.go` already untagged; integration file kept tagged
      for network-ish multi-repo paths)
- [x] 각 신규 테스트는 **뮤테이션으로 검증** — 수정을 되돌리면 실제로 FAIL해야 함
      (코멘트에 mutation 힌트 기록; dirty/switch/copy 케이스는 리스트 단언 포함)

## Notes

Acceptance의 마지막 항목이 이 태스크의 핵심이다. 06에는
`TestCheckRepositoryStateExpandsUntrackedDirectories`라는 **아무것도 검증하지 않는**
테스트가 있었다(이번 세션에 `...SeparatesDirtyFromConflicted`로 교체). 테스트를
추가하는 것과 테스트가 무언가를 붙잡는 것은 다르다.

## References

- 유입 태스크: `06-porcelain-parsers-outside-diff-commit.md`
- 관련: `09-porcelain-parser-silently-skips-malformed-records.md` (short record 케이스 소유)
- 관련: `10-porcelain-parsers-outside-pkg-repository.md`
- 이번 세션 참고 구현: `pkg/reposync/diagnostic_worktree_test.go` (픽스처 전제 자체를 단언하는 패턴)
