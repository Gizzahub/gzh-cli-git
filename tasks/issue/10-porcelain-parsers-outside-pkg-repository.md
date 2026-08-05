# ISSUE: `pkg/repository` 밖에 porcelain 재파싱 6곳 — 2곳은 실제 결함

- status: done
- priority: P2
- category: branch, reposync, doctor
- created_at: 2026-08-05T21:45:00+09:00
- affects: v0.7.0
- spawned_from: `/quality:review:session` (06 완료 직후 세션 리뷰)

## Background

태스크 01·06은 `pkg/repository` **안**의 porcelain 파서 4곳을 `porcelain.go` 하나로
통합했다. 패키지 밖에는 손대지 않았다. 리뷰에서 전수 조사한 결과 6곳이 남아 있고,
그중 2곳은 통합 이전 `pkg/repository`가 갖고 있던 것과 **같은 계열의 실제 결함**이다.

## Findings

### 전수 조사 결과

최초 판정과 재전수조사(작업 완료 시점) 결과를 같이 둔다. 재조사에서 4·6의 판정이
바뀌었다 — porcelain 파싱은 실제로 무해했지만, **같은 줄에 다른 계열의 fail-open이
있었다.**

| # | 위치 | 최초 판정 | 재조사 결과 |
|---|---|---|---|
| 1 | `pkg/branch/parallel.go:279` `getModifiedFiles` | 실제 결함 | 수정 `9b3bf9d` — 호출 사슬의 fail-open 4곳 동반 수정 |
| 2 | `pkg/reposync/executor_git.go:366` | 실제 결함 | 수정 `b0caf10` — `AA`/`DD` + `StatusErr` |
| 3 | `pkg/doctor/repo_checks.go:223,254` | 준결함 | 수정 `7d1fc41` — 에러가 "체크 통과"로 읽히던 것 확인 |
| 4 | `pkg/branch/worktree.go:359` | 결함 아님 | **판정 변경** — porcelain은 무해하나 `Run`의 `ExitCode` 미검사. 파일 전체 5곳 동일. 수정 `7eecc6c` |
| 5 | `pkg/branch/parallel.go:270` | 결함 아님 | 유지 — `ExitCode` 검사는 `9b3bf9d`에서 이미 추가 |
| 6 | `pkg/workspacecli/sync_command.go:1366` | 결함 아님 | 함수는 유지(`cmd.Output()`은 exit code를 에러로 준다). 단 호출자 `:1695`가 `err == nil &&`로 삼킴 — 수정 `7eecc6c` |
| (참고) | `pkg/repository/bulk_stash.go:275` | — | **태스크 08 소관** (미착수) |

"불리언 dirty만 본다"는 판단 자체는 4·5·6 모두에서 옳았다. 놓친 것은 **그 불리언을
읽어내는 방법**이었다: `gitcmd.Executor.Run`은 실패한 git을 `ExitCode`로만 알리므로
`err`만 보는 코드는 모든 git 실패를 성공으로 받는다. 결과는 항상 "비어있음" = "깨끗함".

### 결함 1 — `pkg/branch/parallel.go:279` `getModifiedFiles`

```go
result, err := p.executor.Run(ctx, path, "status", "--porcelain")  // ← ExitCode 미검사
...
    line = strings.TrimSpace(line)
    if len(line) > 2 {
        filename := strings.TrimLeft(line[2:], " \t")
```

한 함수에 네 가지가 겹쳐 있다.

- **rename이 파일명 하나로 뭉개진다.** `R  old.txt -> new.txt`에서 `"old.txt -> new.txt"`를
  파일명으로 반환한다. 존재하지 않는 경로다.
- **`??`가 modified로 계산된다.** 함수 이름과 반환값이 어긋난다.
- **`-uall` 없음** → untracked 디렉터리가 `dir/` 하나로 축약돼 N개가 1개로 보고된다.
- **`-z` 없음** → 공백·비ASCII 경로가 C-quoted 문자열로 나와 실제 파일과 매칭되지 않는다.
- ~~`TrimSpace` 후 `line[2:]`로 경로가 1바이트 밀린다~~ — **오판정. §1 참조.**

소비자: `pkg/branch/parallel.go:182` (`context.ModifiedFiles` 순회) — 단 이 지점 자체가
도달 불가다. D2 참조.

### 결함 2 — `pkg/reposync/executor_git.go:366`

```go
if out, err := cmd.Output(); err == nil {          // ← fail-open
    ...
    if len(line) >= 2 && line[0] == 'U' || (len(line) >= 2 && line[1] == 'U') {
```

- **`AA`·`DD`를 놓친다.** unmerged 코드는 7종(`DD AU UD UA DU AA UU`)인데 `U`가 없는
  `AA`(both added)·`DD`(both deleted)가 조건을 통과하지 못한다.
  `pkg/repository/changeset.go`의 `isUnmergedCode`가 이미 정답을 갖고 있다.
- **`err == nil` fail-open.** git이 죽으면 `IsDirty=false`·`HasConflicts=false`로
  남는다. 태스크 08과 같은 계열.

## Scope

1. `getModifiedFiles`를 `pkg/repository`의 공용 경로로 옮기거나, 최소한
   `-z -uall` + XY 보존 + rename/`??` 분리로 재작성.
2. `executor_git.go`의 충돌 판정을 `isUnmergedCode`와 같은 집합으로 교정하고
   `cmd.Output()` 실패를 삼키지 않게 한다.
3. `pkg/doctor/repo_checks.go:223,254` — 에러 시 `return nil`이 "체크 통과"로 읽히는지
   확인하고, 그렇다면 체크 실패로 표면화.
4. ~~4·5·6은 수정하지 않는다~~ — 주석 근거는 남기되, 재조사에서 4·6은 실제 수정
   대상으로 바뀌었다(위 표).

## Acceptance Criteria

- [x] `getModifiedFiles`가 rename에 대해 실존 경로(신규 경로)만 반환
      — `TestGetModifiedFiles`가 반환 경로 전부에 `os.Lstat`을 건다
- [x] `getModifiedFiles`가 `??`를 modified에 포함하지 않음
      — `TestGetModifiedFilesExcludesUntracked`
- [x] `AA`·`DD` 픽스처에서 `executor_git.go`가 `HasConflicts=true`
      — `TestCollectPostSyncStatusDetects{AA,DD}Conflict`
- [x] `git status` 실패 시 `executor_git.go`가 "깨끗함"으로 판정하지 않음
      — `PostSyncStatus.StatusErr` + 렌더러 2곳 `unknown`
- [x] 4·5·6에 판단 근거 주석 (4·6은 주석과 함께 실제 수정도 필요했다)
- [x] `pkg/repository` 밖 porcelain 파싱 지점 재전수조사 후 이 표 갱신

## 검증 정정 및 보강 (2026-08-05 저녁, 병행 세션)

### 1. 결함 1의 "`TrimSpace` 후 경로 1바이트 밀림"은 **사실이 아니다** ❌

같은 추출 로직을 실제 porcelain 라인에 돌려보면 `" M a.txt"` · `" D a.txt"` ·
`"M  a.txt"` · `"MM a.txt"` 모두 `a.txt`로 정상 추출된다. `TrimSpace`가 X 컬럼의 선행
공백을 정확히 1바이트 지우고 뒤의 `TrimLeft`가 나머지를 흡수하므로 **자기교정된다.**
06의 2-a가 깨진 이유는 그쪽이 컬럼 오프셋(`line[0]`, `line[3:]`)으로 읽었기 때문이고,
여기는 "앞 2글자 버리기"라 같은 함정에 걸리지 않는다.

→ **AC에서 "` M` 경로 밀림" 회귀 테스트는 빼야 한다** (없는 버그를 고정하게 된다).
결함 1의 실제 범위는 **rename · C-quoting · `??` 오분류 · `-uall` 축약 4종**이다.

### 2. 결함 2의 조건식은 결함이 아니다

`len(line) >= 2 && line[0] == 'U' || (len(line) >= 2 && line[1] == 'U')`는 Go 우선순위상
`(a&&b) || (c&&d)`로 묶여 **동작이 맞다.** 두 번째 절이 길이를 다시 검사하기 때문.
가독성 문제일 뿐이므로 수정 시 괄호만 보강하고, 실제 결함은 `AA`/`DD` 누락 하나로 좁힌다.

### 3. 심각도 근거 — `ps.HasConflicts` 소비자는 표시 2곳뿐

`executor_git.go:399`(badge)와 `sync_progress_tui.go:429`(TUI). 의사결정 게이트가
아니므로 코드 영향만 보면 P3 급이다. P2 유지 근거는 **"충돌 없음으로 보이는 화면을
믿고 사용자가 다음 sync를 진행한다"**로 명시해 둔다.

### 4. Scope 1을 실행하려면 결정이 먼저다 → **안 A 채택** (아래 D1)

### 5. `internal/parser`는 패키지 전체가 죽은 코드다

06이 `ParseStatus`를 지웠지만 **남은 7개 함수도 프로덕션 호출자가 0건**이다.
`pkg/doctor`는 `parseAheadBehind` 사본을 따로 갖고 있고 그쪽이 쓰인다 — 06이
`ParseStatus`에서 본 것과 같은 구도.

## Decisions

### D1. 공유 파서 위치 — 안 A (`internal/porcelain`) 채택 · `174c0ab`

취향이 아니라 배제로 정해졌다.

- **안 C 배제**: `pkg/reposync/CLAUDE.md`가 명시적으로 금지한다 — "DON'T call
  `pkg/repository` from executor — use `executor_git.go` (gitcmd direct)".
- **안 B 배제**: `internal/parser`는 호출자 0건인 죽은 패키지다(§5). 죽은 코드를
  되살려 공용 위치로 삼는 것은 정리 대상을 늘린다.
- **안 A 채택**: `internal/`은 컴파일러가 강제하는 가시성 경계라 공개 API 호환 의무가
  없다. 실험적 위치로 두었다가 형태가 굳으면 옮길 수 있다.

**공유 범위는 레코드 수준에서 끊었다.** `Parse`는 `[]Record`(Code/Path/OldPath)까지만
만들고 투영은 각 패키지가 한다. 두 투영이 서로 어긋나기 때문이다 — `Status`는 XY 쌍을
파일 목록의 합집합으로 접고, 충돌 판정은 XY 쌍이 온전해야 한다. 투영까지 공유하면
한쪽은 반드시 정보를 잃는다.

### D2. `ParallelWorkflow`은 프로덕션 소비자가 없다

이 태스크 §3은 결함 1의 소비자로 `pkg/branch/parallel.go:182`를 들었지만 그 지점이
도달 불가다. `pkg/branch` 밖의 참조를 전수 확인한 결과 `ParallelWorkflow`
`GetActiveContexts` `DetectConflicts`는 어디서도 호출되지 않는다.

그래도 고쳤다 — `pkg/branch`는 공개 패키지다. 다만 **심각도 근거를 "사용자 영향"에서
"공개 API 정확성"으로 정정**한다.

### D3. `AA`/`DD` 회귀 테스트는 `U` 부재를 함께 단언한다

`TestCollectPostSyncStatusDetectsAAConflict`는 픽스처가 만든 코드 집합에 `U`가 하나도
없음을 먼저 단언한다. 이것이 없으면 옛 로직으로도 통과할 수 있어 회귀 테스트 구실을
못 한다. `DD` 쪽은 rename/rename 충돌이 `AU`/`UA`를 함께 만들어 같은 단언을 못 걸며,
따라서 회귀 가드가 아니라 커버리지다.

## Follow-ups

- 태스크 `12` — `internal/parser` 전체 삭제 (§5)
- 태스크 `13` — `pkg/branch/{manager,cleanup}.go`의 `Run`+`ExitCode` 미검사 잔여 10곳
- 태스크 `14` — `worktreeManager.Get`의 `filepath.Abs`가 심볼릭 링크를 안 풀어
  macOS `/var` 하위 워크트리를 못 찾는다. 13과 분리한 이유: 오류 보고가 아니라 경로
  해석 결함이고 실측 재현이 이미 있다
- 태스크 `15` — 13 조사 중 파생. `cleanup branch --gone` 무동작

## References

- 선행: `01-changeset-unify-diff-commit-scope.md`, `06-porcelain-parsers-outside-diff-commit.md`
- 관련: `08-conflict-guard-fail-open-on-git-failure.md` (`executor.Run` fail-open 계열)
- 정답 구현: `pkg/repository/porcelain.go`, `pkg/repository/changeset.go` (`isUnmergedCode`)
