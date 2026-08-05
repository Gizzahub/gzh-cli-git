# ISSUE: `gz-git cleanup branch --gone`이 아무것도 하지 않는다

- status: todo
- priority: P2
- category: branch
- created_at: 2026-08-06T10:30:00+09:00
- affects: v0.7.0+
- spawned_from: 태스크 13 조사 중 `isBranchOrphaned` 도달성 확인

## Background

태스크 13에서 `cleanup.go:248`의 fail-open(`git remote` 실패 → 모든 원격추적
브랜치를 "고아"로 판정)이 실제로 삭제까지 도달하는지 확인하다가, **그 지점이
애초에 도달 불가능**하다는 것을 발견했다.

## Findings

### 접두어 검사가 항상 거짓이다

`pkg/branch/cleanup.go:233`

```go
func (c *cleanupService) isBranchOrphaned(ctx context.Context, repo *repository.Repository, branch string) (bool, error) {
	// Remote branches should start with "remotes/"
	if !strings.HasPrefix(branch, "remotes/") {
		return false, nil
	}
```

호출부는 `cleanup.go:115`:

```go
if branch.IsRemote {
    if orphaned, err := c.isBranchOrphaned(ctx, repo, branch.Name); err == nil && orphaned {
```

그런데 `branch.Name`에는 `remotes/`가 **없다.** `parseBranchLine`이
`manager.go:373`에서 이미 떼어낸다:

```go
if strings.HasPrefix(branch.Name, "remotes/") {
    branch.IsRemote = true
    branch.Name = strings.TrimPrefix(branch.Name, "remotes/")   // ← 여기
    branch.Ref = fmt.Sprintf("refs/remotes/%s", branch.Name)
}
```

`git branch -vv --all`의 `remotes/origin/feature` → `branch.Name = "origin/feature"`,
`IsRemote = true`.

따라서 `isBranchOrphaned`는 **첫 줄에서 항상 `false, nil`로 빠져나간다.**
`report.Orphaned`는 언제나 비어 있고, `--gone` 플래그는 무동작이다.

접두어를 떼는 코드와 접두어를 검사하는 코드가 **`IsRemote`라는 불리언을 사이에 두고
갈라져 있다** — 호출부는 이미 `branch.IsRemote`로 원격 여부를 판정한 뒤인데,
피호출부가 같은 판정을 문자열 접두어로 한 번 더 하려 든다. 판정 근거가 이미
구조화되어 있는데 문자열로 되돌아간 것이 결함의 형태다.

### 부수 확인: `Delete`도 원격추적 브랜치 이름을 다루지 못한다

접두어 검사를 고쳐 `report.Orphaned`가 채워지더라도, `Execute` →
`manager.Delete`(`manager.go:133`)는 먼저 `Exists`를 호출하고, `Exists`는
`refs/heads/origin/feature`를 확인한다. 원격추적 브랜치에는 그런 ref가 없으므로
`ErrBranchNotFound`로 끝난다. 그리고 그 에러는 `cleanup.go:165`가 버린다(태스크 13 B).

즉 **접두어 검사만 고치면 "고아로 보고되지만 절대 지워지지 않는" 상태**가 된다.
두 가지를 함께 다뤄야 한다.

### 사용자에게 보이는 현재 증상

`--gone`을 켜도 `👻 Gone branches` 절이 뜨지 않는다
(`cmd/gz-git/cmd/cleanup_branch.go:363`은 `len(report.Orphaned) > 0`일 때만 출력).
사용자는 "고아 브랜치가 없다"로 읽는다. 그것이 참인지 거짓인지 이 명령은 한 번도
확인한 적이 없다.

## Scope

1. `isBranchOrphaned`의 `remotes/` 접두어 검사를 제거한다. 호출부가 이미
   `branch.IsRemote`로 걸러 주므로 중복이자 오작동이다. 함수는 `origin/feature`
   형태를 받는다는 것을 시그니처 주석에 못박는다.
2. 원격 이름 추출(`SplitN(..., "/", 2)`)이 새 입력 형태에 맞는지 확인한다.
   `remotes/` 제거 후에는 `TrimPrefix`가 불필요하다.
3. `--gone` 대상의 삭제 경로를 결정한다. 원격추적 브랜치를 지운다는 것은
   `git branch -dr <name>`(로컬 추적 ref 제거)이지 `git push --delete`(서버의 브랜치
   제거)가 아니다. **둘은 되돌릴 수 있는 정도가 완전히 다르다** — 전자는 `fetch`
   한 번이면 복구되고 후자는 서버에서 사라진다. `--gone`은 "원격이 이미 없어진
   브랜치의 잔재를 치운다"는 뜻이므로 전자여야 하고, 이때 `--remote` 플래그와의
   상호작용을 명시적으로 정해야 한다.
4. 3의 결정에 따라 `manager.Delete`가 원격추적 브랜치를 다룰 수 있게 하거나,
   `cleanup.Execute`가 `Orphaned`를 별도 경로로 처리하게 한다.
5. 회귀 방지: `--gone`이 실제 고아 브랜치를 찾아내는 통합 테스트.

## Acceptance Criteria

- [ ] 원격이 삭제된 추적 브랜치가 있는 픽스처에서 `Analyze(IncludeRemote:true)`가
      `report.Orphaned`에 그 브랜치를 담는다
- [ ] 원격이 살아있는 추적 브랜치는 담기지 **않는다** (2번의 과탐지 방지)
- [ ] `--gone --force`가 대상을 실제로 제거하고, 제거 수가 출력에 반영된다
- [ ] `--gone`이 서버의 브랜치를 지우지 않는다는 것을 확인하는 테스트 (Scope 3 결정 고정)
- [ ] `make quality` 통과
- [ ] CHANGELOG 기재 — 플래그가 지금까지 무동작이었다는 사실 포함

## Decisions

### 필요한 결정: `--gone`과 `--remote`의 관계 (Scope 3)

Scope 3은 구현 전에 확정해야 한다. `--gone --remote`를 "서버의 브랜치도 지운다"로
읽으면, 원격이 이미 사라진 브랜치에 대해 서버 삭제를 시도하는 모순이 된다.
후보:

- **(a)** `--gone`은 `--remote`를 무시한다 — 고아의 정의상 지울 서버 브랜치가 없다
- **(b)** `--gone --remote` 조합을 에러로 거부한다
- **(c)** `--gone`은 로컬 추적 ref만, `--remote`는 `Merged`/`Stale`에만 적용

착수자가 (a)~(c) 중 선택하고 근거를 이 절에 기록한다.

## References

- 상위: `13-branch-manager-cleanup-fail-open-on-git-failure.md` (§A `cleanup.go:248`, §B)
- 코드: `pkg/branch/cleanup.go:115`·`:233`, `pkg/branch/manager.go:373`,
  `cmd/gz-git/cmd/cleanup_branch.go:363`
