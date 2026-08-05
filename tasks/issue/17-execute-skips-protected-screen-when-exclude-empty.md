# ISSUE: `cleanup.Execute`가 `Exclude`가 비면 보호 브랜치 검사를 건너뛴다

- status: todo
- priority: P3
- category: branch
- created_at: 2026-08-06T14:45:00+09:00
- affects: v0.7.0+
- spawned_from: 태스크 13 조사 중 `Execute`의 필터 조건 확인

## Background

태스크 13에서 `Execute`의 삭제 루프를 고치다가, 그 앞의 필터가 **조건부로만 실행된다**는
것을 확인했다. 오늘 기준 실제 삭제로 이어지지 않지만, 그것은 `Execute`가 막고 있어서가
아니라 **호출자가 우연히 전부 안전한 입력을 주기 때문**이다.

## Findings

### 필터가 `len(opts.Exclude) > 0`일 때만 돈다

`pkg/branch/cleanup.go` `Execute`:

```go
// Filter out excluded branches
if len(opts.Exclude) > 0 {
    filtered := make([]*Branch, 0)
    for _, branch := range toDelete {
        if !c.isProtectedBranch(branch.Name, opts.Exclude) {
            filtered = append(filtered, branch)
        }
    }
    toDelete = filtered
}
```

`isProtectedBranch`는 **내장 보호 목록(`IsProtected`)과 추가 패턴을 함께** 검사한다:

```go
func (c *cleanupService) isProtectedBranch(branch string, additionalPatterns []string) bool {
	if IsProtected(branch) {          // main, master, develop, development, release/*, hotfix/*
		return true
	}
	for _, pattern := range additionalPatterns { ... }
}
```

즉 `Exclude`는 **추가** 패턴인데, 그것이 비어 있으면 내장 목록 검사까지 통째로 건너뛴다.
`--protect`를 주지 않은 호출에서 `main`이 `toDelete`에 들어와 있으면 그대로 삭제된다.

### 오늘 도달 불가능한 이유 — 그리고 그것이 왜 보증이 아닌지

`Analyze`가 보호 브랜치를 `report.Protected`로 빼돌리고 `Merged`/`Stale`/`Orphaned`에
넣지 않으므로, `Analyze`가 만든 보고서를 그대로 넘기는 세 호출자
(단일 CLI·벌크 CLI·`pkg/wizard`)는 안전하다.

그러나 `Execute`는 `pkg/`의 **공개 API**이고, `*CleanupReport`도 공개 구조체다.
보고서를 직접 조립하는 호출자는 아무 검사도 받지 못한다 — 그리고 시그니처는
`ExecuteOptions.Exclude`가 "추가 제외"라고 말하지 "이걸 비우면 내장 보호가 꺼진다"고
말하지 않는다. **방어의 위치가 틀렸다**: 삭제 직전에 있어야 할 검사가 선택적 필터
안에 들어가 있다.

태스크 15가 `Orphaned`를 실제로 채우기 시작했으므로 `toDelete`에 들어오는 경로가
하나 늘었다는 점도 있다(그 경로 역시 `Analyze`를 거치므로 오늘은 안전하다).

## Scope

1. 내장 보호 검사를 `len(opts.Exclude)` 조건 **밖으로** 꺼낸다. 필터는 항상 돈다.
2. 걸러낸 브랜치를 조용히 버리지 않는다. 태스크 13이 `ExecuteResult`를 도입했으므로
   `Skipped []string`(또는 `Failed`에 사유를 담은 항목)으로 호출자에게 보인다 —
   지금은 "요청했는데 안 지워졌다"가 아무 데도 안 나타난다.
3. 회귀 테스트: `Exclude`가 빈 `ExecuteOptions`로 `Merged: ["main"]`을 넘겨도
   `main`이 살아남는다.
4. `Analyze`가 이미 걸러 준다는 사실에 기대지 않음을 주석으로 남긴다.

## Acceptance Criteria

- [ ] `Execute(ctx, repo, &CleanupReport{Merged: []*Branch{{Name:"main"}}}, ExecuteOptions{Force:true, Confirm:true})`가 `main`을 지우지 않는다
- [ ] 걸러진 브랜치가 반환값에 드러난다
- [ ] 기존 3개 호출 경로의 동작 불변 (`Analyze` 보고서를 넘기면 결과가 같다)
- [ ] `make quality` 통과

## Decisions

### D1. P3인 이유 — 그리고 P3이 "무시해도 된다"는 뜻이 아닌 이유

오늘 실제 데이터 손실로 이어지는 경로가 없으므로 P0/P2가 아니다. 다만 이 안전은
`Execute` 자신이 아니라 **모든 호출자가 `Analyze`를 거친다는 우연**에 의존한다.
새 호출자 하나가 그 규칙을 모르면 `main`이 지워지고, 그 시점에는 P0다.

## References

- 코드: `pkg/branch/cleanup.go` (`Execute`의 필터, `isProtectedBranch`),
  `pkg/repository/branch_protect.go` (`ProtectedBranches`, `IsProtected`)
- 상위: `13-branch-manager-cleanup-fail-open-on-git-failure.md` (`ExecuteResult` 도입)
- 인접: `15-cleanup-gone-flag-is-a-no-op.md` (`Orphaned`가 실제로 채워지기 시작한 변경)
