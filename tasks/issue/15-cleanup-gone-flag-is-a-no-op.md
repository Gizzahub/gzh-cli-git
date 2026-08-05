# ISSUE: `gz-git cleanup branch --gone`이 아무것도 하지 않는다

- status: done
- priority: P2
- completed_at: 2026-08-06T14:20:00+09:00
- category: branch
- created_at: 2026-08-06T10:30:00+09:00
- affects: v0.7.0+
- spawned_from: 태스크 13 조사 중 `isBranchOrphaned` 도달성 확인

## Background

태스크 13에서 `cleanup.go:248`의 fail-open(`git remote` 실패 → 모든 원격추적
브랜치를 "고아"로 판정)이 실제로 삭제까지 도달하는지 확인하다가, **그 지점이
애초에 도달 불가능**하다는 것을 발견했다.

착수 후 결함이 하나가 아니라 **셋**이며, 그중 가장 큰 것은 접두어 버그가 아니라
**"고아"의 정의 자체가 틀렸다**는 것으로 드러났다. Findings의 §0이 그 발견이다.

## Findings

### 0. 같은 개념의 구현이 둘이고, 서로 다른 것을 가리킨다 ★

`--gone`에는 구현이 두 벌 있다. 디렉터리 인자를 주면 벌크 경로, 안 주면 단일 경로로
갈라지는데(`cleanup_branch.go:103`) **두 경로가 서로 다른 질문에 답한다.**

| | 벌크 (`pkg/repository/bulk_cleanup.go:462`) | 단일 (`pkg/branch/cleanup.go` 구 `isBranchOrphaned`) |
|---|---|---|
| 대상 | **로컬** 브랜치 (`refs/heads/`) | **원격추적** 브랜치 (`refs/remotes/`) |
| 판정 | `for-each-ref %(upstream:track)`의 `[gone]` | `git remote` 목록에 remote 이름이 있는가 |
| 근거 | git 자신의 답 | 직접 만든 규칙 |
| 삭제 | `git branch -D` | (도달 불가) |

git이 `[gone]`이라 부르는 것, `git branch -vv`가 `[origin/x: gone]`으로 찍는 것,
사용자가 "고아 브랜치"라 말하는 것은 전부 **왼쪽**이다. 오른쪽은 "등록되지 않은
remote를 가리키는 추적 ref"로, 이름만 같지 다른 개념이다.

따라서 이 태스크의 수정은 접두어 검사를 고치는 것이 아니라 **오른쪽 구현을 버리고
왼쪽으로 통일**하는 것이다. 프로젝트가 이미 답을 갖고 있었고, 한쪽이 그것을 몰랐다.

### 1. 접두어 검사가 항상 거짓이다 (원래 발견)

`pkg/branch/cleanup.go` 구 `isBranchOrphaned`:

```go
// Remote branches should start with "remotes/"
if !strings.HasPrefix(branch, "remotes/") {
    return false, nil
}
```

`branch.Name`에는 `remotes/`가 **없다.** `parseBranchLine`이 `manager.go:373`에서
이미 떼어낸다:

```go
if strings.HasPrefix(branch.Name, "remotes/") {
    branch.IsRemote = true
    branch.Name = strings.TrimPrefix(branch.Name, "remotes/")   // ← 여기
```

접두어를 떼는 코드와 접두어를 검사하는 코드가 **`IsRemote`라는 불리언을 사이에 두고
갈라져 있다** — 호출부는 이미 `branch.IsRemote`로 판정한 뒤인데, 피호출부가 같은
판정을 문자열로 한 번 더 하려 든다. 판정 근거가 이미 구조화되어 있는데 문자열로
되돌아간 것이 결함의 형태다.

### 2. `AnalyzeOptions`에 `IncludeGone`이 없었다

`--gone`이 도달할 필드 자체가 없었다. `AnalyzeOptions`는 `IncludeMerged`/
`IncludeStale`/`IncludeRemote`만 갖고 있었고, 고아 판정은 `IncludeRemote`에
얹혀 있었다 — `--remote` 없이 `--gone`만 주면 원격 브랜치를 나열조차 하지 않으니
1번을 고쳐도 여전히 무동작이다.

### 3. 단일 경로 CLI가 `--gone`을 `Analyze`에 넘기지 않았다

`runSingleRepoCleanupBranch`의 `analyzeOpts`에 고아 관련 필드가 없었다.
`runBulkCleanupBranch`(`:207`)는 `IncludeGone`을 넘긴다. **플래그가 구현된 것처럼
보인 이유**가 이것이다 — 디렉터리 인자를 준 사람에게는 동작했다.

세 결함은 독립이다. 어느 하나만 고쳐도 `--gone`은 여전히 무동작이다.

### 사용자에게 보이는 증상

`--gone`을 켜도 `👻 Gone branches` 절이 뜨지 않는다
(`printCleanupBranchReport`는 `len(report.Orphaned) > 0`일 때만 출력).
사용자는 "고아 브랜치가 없다"로 읽는다. 그것이 참인지 거짓인지 이 명령은 한 번도
확인한 적이 없다.

## Scope

1. ~~`isBranchOrphaned`의 `remotes/` 접두어 검사를 제거한다~~ → **함수를 삭제**하고
   `findGoneBranches`로 대체한다. 벌크 경로와 같은 방식:
   `fetch --prune` 후 `for-each-ref --format=%(refname:short) %(upstream:track)
   refs/heads/`에서 `[gone]` 줄을 고른다.
2. ~~원격 이름 추출이 새 입력 형태에 맞는지 확인~~ → 해당 없음(함수 삭제).
3. `--gone`과 `--remote`의 관계를 결정한다 (아래 D1).
4. ~~`manager.Delete`가 원격추적 브랜치를 다룰 수 있게 한다~~ → **불필요.**
   고아는 로컬 브랜치이므로 `manager.Delete`가 이미 다룬다. 이 항목은 "고아 =
   원격추적 브랜치"라는 §0의 잘못된 전제에서 나온 것이다.
5. `AnalyzeOptions.IncludeGone` 추가 + 단일 경로 CLI 배선 (§2·§3).
6. 회귀 방지 테스트.
7. CHANGELOG 기재.

## Acceptance Criteria

- [x] 상위가 삭제된 브랜치가 있는 픽스처에서 `Analyze(IncludeGone:true)`가
      `report.Orphaned`에 그 브랜치를 담는다 —
      `TestCleanupService_AnalyzeFindsBranchWithGoneUpstream`
- [x] 상위가 살아있는 브랜치는 담기지 **않는다** (과탐지 방지) — 같은 테스트의
      `live-upstream` assertion
- [x] `IncludeGone:false`면 `Orphaned`가 빈다 (플래그가 실제로 동작을 가른다) —
      `TestCleanupService_AnalyzeSkipsGoneWhenNotRequested`
- [x] `--gone --force`가 대상을 실제로 제거하고 제거 수가 출력에 반영된다 —
      `TestCleanupBranchGoneDeletesAndCounts` (`✓ Deleted 1 branch(es)`)
- [x] `--gone`이 서버의 브랜치를 지우지 않는다 (D1 고정) —
      `TestCleanupService_ExecuteGoneLeavesOriginUntouched`
- [x] `make quality` 통과 (`make lint` 신규 지적 0건 — 아래 D3)
- [x] CHANGELOG 기재 — 플래그가 지금까지 무동작이었다는 사실 포함

## Decisions

### D1. `--gone`과 `--remote`의 관계 → **(c)**, 선택이 아니라 확인

원안은 (a)~(c) 중 착수자가 고르는 것이었다. 고를 필요가 없었다 — §0에서 프로젝트가
이미 (c)를 구현하고 있음이 드러났다. 벌크 경로의 `--gone`은 로컬 브랜치를
`git branch -D`로 지우고 서버를 건드리지 않는다. 단일 경로를 거기 맞추는 것이
이 태스크의 일이지, 새 정책을 세우는 것이 아니다.

**(c)**: `--gone`은 로컬 ref만 지운다. `--remote`는 `Merged`/`Stale`에만 적용된다.

강제할 코드는 필요 없다. `Execute`가 `Remote: opts.Remote && branch.IsRemote`로
계산하는데 고아는 로컬이라 `IsRemote == false`이므로, `--remote`를 켜도
`git push --delete`가 발화할 수 없다. 이 성질을 테스트로 못박았다 —
`ExecuteGoneLeavesOriginUntouched`가 `Remote: true`로 실행하고 origin의 브랜치
목록이 그대로임을 확인한다.

되돌릴 수 있는 정도가 완전히 다르다는 원안의 지적은 그대로 유효하다. 로컬 ref는
reflog로 복구되지만 서버 브랜치는 사라진다. (c)는 그 비대칭을 코드 구조로 보장한다.

### D2. `nonInteractiveEnv`를 `pkg/repository`에서 공개한다

`findGoneBranches`의 `fetch --prune`도 자격증명 프롬프트에 막히면 안 된다.
`pkg/repository`에 이미 같은 목록이 있는데 비공개였다. 복제하면 §0과 같은 종류의
분기가 하나 더 생긴다 — git이 프롬프트 채널을 하나 더 갖게 되는 날 한쪽만 고쳐진다.

`repository.NonInteractiveEnv()`로 접근자를 열었다. 값이 아니라 함수인 이유는 슬라이스
전역을 공개하면 호출자가 변경할 수 있기 때문이다(기존 `ProtectedBranches`가 가진
문제이기도 하다). 사본을 돌려주므로 호출자가 자기 항목을 덧붙여도 안전하다.

### D3. `make lint`는 이 커밋 이후 18건 — 전부 미수정 파일

`pkg/config/keyring.go`(9), `pkg/repository/bulk_exec*.go`(5), `goheader`(4).
신규 파일 지적은 0건이다.

`goheader`는 **저장소 전체가 위반**하는 상태다 — 설정은 `Archmagece`를 기대하는데
모든 파일이 `Gizzahub`이라 적혀 있다. 지적 건수가 실행마다 달라지는 것은
golangci-lint의 `max-same-issues` 때문이지 코드가 바뀌어서가 아니다.
`make quality`가 `lint-check`를 제외하므로 아무도 보지 못한다. 별도 태스크 감.

## Verification

`findGoneBranches`의 `[gone]` 검사를 두 방향으로 뒤집어 확인했다.

**항상 빈 집합**(구 동작 재현):

```
AnalyzeFindsBranchWithGoneUpstream: Orphaned = [], want it to contain gone-upstream
ExecuteGoneLeavesOriginUntouched:   Deleted  = [], want it to contain gone-upstream
                                    clone branches = [gone-upstream live-upstream master]
```

**항상 전체 집합**(과탐지):

```
AnalyzeFindsBranchWithGoneUpstream: Orphaned = [gone-upstream live-upstream],
                                    want live-upstream left alone
```

CLI 배선(§3)은 `analyzeOpts`에서 `IncludeGone` 한 줄을 빼면 재현된다:

```
✓ No branches to clean up
gone-upstream survived the deletion
```

`AnalyzeSkipsGoneWhenNotRequested`는 두 변이 모두에서 통과한다 — 재현이 아니라
가드다. 첫 번째 테스트가 "모든 브랜치를 고아로 보고하는" 구현에서도 통과하는 것을
막는 자리다.

## References

- 상위: `13-branch-manager-cleanup-fail-open-on-git-failure.md` (§A `cleanup.go:248`, §B)
- 참조 구현: `pkg/repository/bulk_cleanup.go:462` `getGoneBranches`
  (단, 이쪽도 `Run` 후 `ExitCode` 미검사 — 태스크 08 범위)
- 코드: `pkg/branch/cleanup.go`, `pkg/branch/types.go`,
  `cmd/gz-git/cmd/cleanup_branch.go`, `pkg/repository/bulk.go`
