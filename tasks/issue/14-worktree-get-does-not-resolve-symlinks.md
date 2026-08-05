# ISSUE: `worktreeManager.Get`이 심볼릭 링크를 풀지 않아 워크트리를 못 찾는다

- status: done
- priority: P2
- category: branch
- created_at: 2026-08-06T10:20:00+09:00
- affects: v0.7.0+ (심볼릭 링크 하위 경로 전부 — macOS `/tmp`·`/var` 포함)
- spawned_from: `10-porcelain-parsers-outside-pkg-repository.md` Follow-ups

## Background

태스크 10 Scope 4의 회귀 테스트를 쓰다가 걸렸다. `TestWorktreeManager_RemoveRefusesDirtyWorktree`가
정상적으로 생성된 워크트리에 대해 실패했다:

```
Add() error = failed to get worktree info: worktree not found: /var/folders/.../feature-wt
```

**실측으로 확인된 결함이다.** 테스트는 `filepath.EvalSymlinks`로 우회했고
(`worktree_failure_test.go:64`), 근본 원인은 남겨 이 태스크로 분리했다.

## Findings

`pkg/branch/worktree.go:253` `Get`

```go
absPath, err := filepath.Abs(path)          // :263  호출자가 준 경로
...
for _, wt := range worktrees {
    wtAbsPath, err := filepath.Abs(wt.Path) // :276  git이 보고한 경로
    if wtAbsPath == absPath {               // :281  문자열 비교
```

**`filepath.Abs`는 심볼릭 링크를 풀지 않는다.** 상대 경로를 절대 경로로 만들고
`.`/`..`를 정리할 뿐이다. 그런데 `git worktree list`는 **이미 해석된 경로**를
보고한다. 그래서 비교의 양변이 서로 다른 표기로 같은 디렉터리를 가리킨다.

| | 값 |
|---|---|
| 호출자가 준 경로 | `/var/folders/xy/.../feature-wt` |
| git이 보고한 경로 | `/private/var/folders/xy/.../feature-wt` |
| `filepath.Abs` 처리 후 | **둘 다 그대로** — 일치하지 않는다 |

macOS는 `/var` → `/private/var`, `/tmp` → `/private/tmp`가 기본이다. 리눅스에서도
홈 하위를 외부 볼륨에 심볼릭 링크로 붙이는 배치는 흔하다. 예외 상황이 아니다.

### 영향

| 호출 지점 | 결과 |
|---|---|
| `worktree.go:148` (`Add`) | **워크트리는 실제로 생성된 뒤** `Get`이 실패해 `Add`가 에러를 반환한다. 사용자에게는 실패로 보이지만 디스크에는 남는다 — 가장 나쁜 형태 |
| `worktree.go:167` (`Remove`) | 존재하는 워크트리에 `ErrWorktreeNotFound`. **CLI로 지울 방법이 없어진다** |
| `worktree.go:299` (`Exists`) | 존재하는 워크트리에 `false` |
| `parallel.go:142` | 대상 워크트리를 못 찾아 작업 자체가 시작되지 않음 |

`gz-git worktree add/remove`(`cmd/gz-git/cmd/worktree.go`)가 전부 이 경로를 탄다.

## Scope

1. `Get`의 비교를 `filepath.EvalSymlinks` 기반으로 바꾼다. 양변 모두 풀어야 한다 —
   한쪽만 풀면 반대 방향(호출자가 이미 해석된 경로를 준 경우)이 깨진다.
2. **`EvalSymlinks`는 존재하지 않는 경로에 에러를 낸다.** `Get`은 "없는 워크트리"를
   정상적으로 다뤄야 하므로(`Exists`가 이 위에 있다), 해석 실패 시 `Abs` 결과로
   폴백하고 그 값으로 비교를 계속한다. 해석 실패를 `Get` 실패로 바꾸면
   `Exists(없는경로)`가 `false, nil` 대신 에러가 되어 계약이 바뀐다.
3. `wt.Path` 쪽 해석 실패는 현행처럼 `continue`가 아니라 **폴백 후 비교**로 바꾼다.
   지금은 해석 불가 항목을 목록에서 조용히 지운다.
4. 헬퍼를 하나 두고 `Get`이 그것만 쓰게 한다 — 양변에 같은 정규화를 적용한다는 점이
   이 수정의 전부이므로, 두 곳에 복제하면 다음에 한쪽만 고쳐진다.
5. `worktree_failure_test.go:64`의 `EvalSymlinks` 우회를 걷어낸다. 그 우회가
   남아 있으면 테스트가 결함을 다시 가려 준다.

## Acceptance Criteria

- [x] 심볼릭 링크 하위에 워크트리를 만들고 `Add`가 nil error + 유효한 `*Worktree`를
      반환 — `TestWorktreeManager_AddThroughSymlinkedPath`
- [x] 같은 경로로 `Exists`가 `true`, `Remove`가 성공 —
      `..._GetMatchesEitherSpelling`, `..._RemoveThroughSymlinkedPath`
- [x] 해석된 경로로도 동일 동작 — `..._GetMatchesEitherSpelling` 의 두 서브테스트
- [x] 존재하지 않는 경로에 `Exists`가 `false, nil` — `..._ExistsOnAbsentPath`
- [x] `worktree_failure_test.go`의 `EvalSymlinks` 우회 제거
- [x] 뮤테이션 검증: `EvalSymlinks` 호출을 항등으로 되돌리면 4건이
      `worktree not found: /var/folders/...` 로 실패. `ExistsOnAbsentPath`는 통과 —
      그 테스트는 링크 해석에 의존하지 않으므로 올바른 결과다
- [x] `make quality` 통과
- [x] CHANGELOG 기재

## Decisions

### D1. Scope 1~4를 `samePath` 하나로 합쳤다

태스크는 헬퍼를 두라고만 했지만(Scope 4), 실제 형태는 "정규화 함수"가 아니라
**비교 함수**여야 했다. 정규화 함수는 값 하나를 받아 값 하나를 돌려주므로 폴백
(Scope 2)을 표현할 수 없다 — 해석 실패 시 어떤 값을 돌려주든 반대편이 무엇으로
정규화됐는지 모른 채 비교당한다. `samePath(a, b) bool`은 양변을 함께 보므로
"철자가 같으면 즉시 참, 아니면 양쪽 해석 후 비교, 한쪽이라도 해석 불가면 거짓"을
한 곳에 담는다.

### D2. 철자 비교를 먼저 두어 Scope 3의 회귀를 막았다

`absA == absB`를 앞에 둔 것은 최적화가 아니다. **디렉터리가 삭제된 워크트리**는
`git worktree list`에 여전히 나타나고(prunable), 그것을 지우는 것은 호출자가 실제로
하는 일이다. 그런 경로는 `EvalSymlinks`가 실패하므로, 철자 비교가 없으면 해석
단계에서 거짓이 되어 **지울 방법이 사라진다.** 기존 코드의 `continue`가 우연히
막아 주던 것을 명시적 분기로 바꿨다.

### D3. `Get`의 `filepath.Abs` 에러 검사는 남겼다

`samePath` 안에서도 `Abs`를 부르지만, 호출자가 준 경로의 `Abs` 실패
(`os.Getwd()` 실패)는 "워크트리 없음"이 아니라 환경 오류다. `Get` 상단의 검사를
지웠다면 그 구분이 사라진다. 중복 호출 1회의 비용보다 계약 보존이 크다.

## References

- 실측 재현: 태스크 10 작업 중, 커밋 `7eecc6c`의 `worktree_failure_test.go`
- `Get` 호출자 4곳: `worktree.go:148`·`:167`·`:299`, `parallel.go:142`
