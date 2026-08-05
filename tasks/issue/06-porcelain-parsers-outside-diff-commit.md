# ISSUE: diff/commit 밖에 동일한 porcelain 파싱 결함이 2곳 더 존재

- status: todo
- priority: P3
- category: repository
- created_at: 2026-08-05T16:00:00+09:00
- affects: v0.7.0
- spawned_from: `01-changeset-unify-diff-commit-scope.md` (Follow-up #2)

## Background

태스크 01이 `BulkDiff`/`BulkCommit`의 인라인 porcelain 파서를 `collectChangeSet`
(`--porcelain -z -uall`)으로 통합했지만, **같은 결함 계열이 diff/commit 밖에 2곳 더
남아 있다.** 둘 다 `git status --porcelain`을 `-z`/`-uall` 없이 호출해, 01이 고친 것과
같은 세 증상(디렉터리 축약·C-quoting·충돌 판정 중복)을 재현한다.

범위를 01에서 분리한 이유: 이 두 경로는 커밋 경로가 아니라 **상태/헬스 표시**에 쓰이므로
즉시 비가역 손상(02)이나 증거-불일치(01)를 일으키지 않는다. 심각도가 다르므로 별도 태스크.

## Findings

### 1. `pkg/repository/bulk.go:1813` — `checkRepositoryState`

```go
statusResult, err := c.executor.Run(ctx, repoPath, "status", "--porcelain")   // -z/-uall 없음
...
lines := strings.Split(strings.TrimSpace(statusResult.Stdout), "\n")
state.UncommittedFiles = len(lines)                                            // ?? docs/ → 1건으로 축약
...
status := line[:2]
if strings.Contains(status, "U") || status == "AA" || status == "DD" {         // isUnmergedCode 재구현
    state.HasConflicts = true
    state.ConflictedFiles = append(state.ConflictedFiles, strings.TrimSpace(line[3:]))  // C-quoted 경로
}
```

- **`UncommittedFiles = len(lines)`** — untracked 디렉터리가 `?? docs/` 1라인으로 축약되어
  실제 파일 수(N)가 아닌 1로 계상된다. `01`의 `commit-preview-undercounts-untracked-dirs`와 동일 근원.
- **충돌 판정 재구현** — `isUnmergedCode`(`pkg/repository/changeset.go:426`)가 이미 존재함에도
  `strings.Contains(status,"U") || =="AA" || =="DD"`로 다시 짰다. (현재 기능적으로 등가 —
  7개 unmerged 코드 DD AU UD UA DU AA UU를 모두 커버 — 이지만 분기가 분산되어 유지비용의 원인)
- **`line[3:]` 경로** — porcelain v1은 경로를 C-quote하므로 공백/비ASCII 경로가 깨진다.
  rename 라인은 `old -> new` 형태라 경로로서 무효.

> 참고: `state.UncommittedFiles`의 소비자를 먼저 조사해야 한다 — 이 값이 헬스 표시용인지
> 의사결정(guard)에 쓰이는지에 따라 수정 긴급도가 달라진다.

### 2. `pkg/repository/client.go:447` — `GetStatus` → `parseStatus` (`:501`)

```go
output, err := c.executor.RunOutput(ctx, repo.Path, "status", "--porcelain")  // -z/-uall 없음
...
func parseStatus(output string) (*Status, error) {
    ...
    filePath := strings.TrimSpace(line[3:])                                    // C-quoted 경로
```

- `status --porcelain`을 `-z`/`-uall` 없이 호출. 같은 클래스.
- `parseStatus`는 이미 `internal/parser/status.go:119`에 **중복된** porcelain 분류 로직이
  별도로 존재한다(`01` Background가 지적한 중복의 연장).
- `line[3:]` → C-quoted 경로 방출. rename 처리 분기가 별도로 있으나(`:533`) 비ASCII는 깨짐.

## Scope (확정 아님 — 착수 시 재구성)

1. 두 호출을 `--porcelain -z -uall`로 전환하거나, `collectChangeSet`/`isUnmergedCode`를
   재사용해 파싱을 단일 경로로 모은다.
2. `parseStatus` ↔ `internal/parser/status.go` 중복 제거(둘 중 하나를 단일 소스로).
3. `checkRepositoryState`의 충돌 판정을 `isUnmergedCode` 호출로 대체.
4. `UncommittedFiles`의 의미 재확정: "파일 수"인지 "항목 수"인지. 소비자 호환성 확인.

### 하위호환

`Status`/`repositoryState`의 공개 필드값이 바뀔 수 있다(축약 해제로 카운트 증가).
공개 API이므로 마이너 버전에서 값 변경은 CHANGELOG에 breaking-fix로 명시.

## Acceptance Criteria (착수 시 확정)

- [ ] `checkRepositoryState`가 `-uall` 없는 porcelain을 직접 파싱하지 않음
- [ ] `parseStatus`가 `-z`/`-uall` 기반 파싱을 사용하거나 단일 파서로 통합됨
- [ ] 충돌 판정이 `isUnmergedCode`를 재사용 (중복 분기 제거)
- [ ] `?? 디렉터리` 축약 케이스에 대한 회귀 테스트 추가
- [ ] C-quoted/비ASCII 경로가 실경로로 반환됨 (01의 `TestChangeSetUnquotesPaths`와 동일 기준)

## References

- 원본 감사: workflow `wf_6c7e7604-0aa`
- 관련 태스크: `01-changeset-unify-diff-commit-scope.md` (동일 결함 계열, 커밋/표시 경로)
