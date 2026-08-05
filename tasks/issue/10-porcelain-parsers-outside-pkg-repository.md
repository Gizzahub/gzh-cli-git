# ISSUE: `pkg/repository` 밖에 porcelain 재파싱 6곳 — 2곳은 실제 결함

- status: todo
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

| # | 위치 | 판정 |
|---|---|---|
| 1 | `pkg/branch/parallel.go:279` `getModifiedFiles` | **실제 결함** (아래) |
| 2 | `pkg/reposync/executor_git.go:366` | **실제 결함** (아래) |
| 3 | `pkg/doctor/repo_checks.go:223,254` | 준결함 — `RunOutput` 에러 시 `return nil`로 체크가 통째로 사라짐 |
| 4 | `pkg/branch/worktree.go:359` | 결함 아님 — 불리언 dirty 판정만, 축약/quoting에 둔감 |
| 5 | `pkg/branch/parallel.go:270` | 결함 아님 — 동일 |
| 6 | `pkg/workspacecli/sync_command.go:1366` | 결함 아님 — 동일 |
| (참고) | `pkg/repository/bulk_stash.go:275` | `_, _ :=` + `ExitCode`만 검사. 방향은 fail-safe지만 근거는 우연 — **태스크 08 소관** |

4·5·6은 "비어있냐 아니냐"만 보므로 디렉터리 축약도 C-quoting도 답을 바꾸지 못한다.
공용 파서로 옮기면 코드는 통일되지만 고쳐지는 버그는 없다. **1·2만 실제 수정 대상.**

### 결함 1 — `pkg/branch/parallel.go:279` `getModifiedFiles`

```go
result, err := p.executor.Run(ctx, path, "status", "--porcelain")
if err != nil {
    return nil, err
}
...
    line = strings.TrimSpace(line)
    ...
    if len(line) > 2 {
        filename := strings.TrimLeft(line[2:], " \t")
        files = append(files, filename)
    }
```

한 함수에 네 가지가 겹쳐 있다.

- **rename이 파일명 하나로 뭉개진다.** `R  old.txt -> new.txt`에서 `"old.txt -> new.txt"`를
  파일명으로 반환한다. 존재하지 않는 경로다.
- **`??`가 modified로 계산된다.** 함수 이름과 반환값이 어긋난다.
- **`-uall` 없음** → untracked 디렉터리가 `dir/` 하나로 축약돼 N개가 1개로 보고된다.
- **`-z` 없음** → 공백·비ASCII 경로가 C-quoted 문자열로 나와 실제 파일과 매칭되지 않는다.
- **`TrimSpace` 후 `line[2:]`** → 워크트리 전용 변경(` M`)은 앞 공백이 잘려 `M `으로
  읽히고 경로가 1바이트 밀린다. `pkg/repository`에서 이미 고친 것과 **같은 결함**이다.

소비자: `pkg/branch/parallel.go:182` (`context.ModifiedFiles` 순회).

### 결함 2 — `pkg/reposync/executor_git.go:366`

```go
cmd = exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain")
if out, err := cmd.Output(); err == nil {
    output := strings.TrimSpace(string(out))
    if output != "" {
        ps.IsDirty = true
        for line := range strings.SplitSeq(output, "\n") {
            if len(line) >= 2 && line[0] == 'U' || (len(line) >= 2 && line[1] == 'U') {
                ps.HasConflicts = true
                break
            }
        }
    }
}
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
4. 4·5·6은 **수정하지 않는다.** 대신 "불리언 dirty만 필요하므로 공용 파서 불필요"를
   주석으로 남겨 다음 조사자가 같은 판단을 반복하지 않게 한다.

## Acceptance Criteria

- [ ] `getModifiedFiles`가 rename에 대해 실존 경로(신규 경로)만 반환
- [ ] `getModifiedFiles`가 `??`를 modified에 포함하지 않음
- [ ] `AA`·`DD` 픽스처에서 `executor_git.go`가 `HasConflicts=true`
- [ ] `git status` 실패 시 `executor_git.go`가 "깨끗함"으로 판정하지 않음
- [ ] 4·5·6에 판단 근거 주석
- [ ] `pkg/repository` 밖 porcelain 파싱 지점 재전수조사 후 이 표 갱신

## 검증 정정 및 보강 (2026-08-05 저녁, 병행 세션)

07 작업 중 같은 지점을 독립 조사하다 겹쳤다. 중복 태스크는 폐기하고 결과만 여기 합친다.

### 1. 결함 1의 "`TrimSpace` 후 경로 1바이트 밀림"은 **사실이 아니다** ❌

같은 추출 로직을 실제 porcelain 라인에 돌린 결과:

| raw | 결과 | 판정 |
|---|---|---|
| `" M a.txt"` (워크트리 전용 수정) | `a.txt` | 정상 |
| `" D a.txt"` (워크트리 전용 삭제) | `a.txt` | 정상 |
| `"M  a.txt"` / `"MM a.txt"` / `"A  a.txt"` | `a.txt` | 정상 |
| `"?? docs/"` | `docs/` | 축약은 사실 (`-uall` 없음) |
| `"R  old.txt -> new.txt"` | `old.txt -> new.txt` | **결함** |
| `" M "\303\251.md""` | `"\303\251.md"` | **결함** |

`TrimSpace`가 X 컬럼의 선행 공백을 정확히 1바이트 지우고 뒤의 `TrimLeft`가 나머지를
흡수하므로 **자기교정된다.** 06의 2-a가 깨진 이유는 그쪽이 컬럼 오프셋(`line[0]`,
`line[3:]`)으로 읽었기 때문이고, 여기는 "앞 2글자 버리기"라 같은 함정에 걸리지 않는다.

→ **AC에서 "` M` 경로 밀림" 회귀 테스트는 빼야 한다** (없는 버그를 고정하게 된다).
결함 1의 실제 범위는 **rename · C-quoting · `??` 오분류 · `-uall` 축약 4종**이다.

### 2. 결함 2의 조건식은 결함이 아니다

`len(line) >= 2 && line[0] == 'U' || (len(line) >= 2 && line[1] == 'U')`는 Go 우선순위상
`(a&&b) || (c&&d)`로 묶여 **동작이 맞다.** 두 번째 절이 길이를 다시 검사하기 때문.
가독성 문제일 뿐이므로 수정 시 괄호만 보강하고, 실제 결함은 `AA`/`DD` 누락 하나로 좁힌다.

### 3. 심각도 근거 — `ps.HasConflicts` 소비자는 표시 2곳뿐

```
pkg/reposync/executor_git.go:399        badge 문자열 "conflict" / "dirty"
pkg/workspacecli/sync_progress_tui.go:429   TUI 표시
```

의사결정 게이트가 아니다. 코드 영향만 보면 P3 급이고, P2를 유지하려면 근거를
"충돌 없음으로 보이는 화면을 믿고 사용자가 sync를 진행한다"로 명시해 두는 게 좋다.

### 4. Scope 1을 실행하려면 결정이 먼저다

`parsePorcelainZ`는 `pkg/repository`의 **비공개** 함수라 `pkg/branch`·`pkg/doctor`·
`pkg/reposync`에서 그대로 못 쓴다. "공용 경로로 옮기거나"가 열려 있는 상태.

| 안 | 내용 | 대가 |
|----|------|------|
| A | `internal/porcelain` 신설, `pkg/repository`가 그것을 사용 | 06 결과물의 이동 diff 발생 |
| B | 비어버린 `internal/parser`를 공유 파서 자리로 재사용 | 06이 "죽은 코드라 삭제"한 위치를 되살리는 형태 |
| C | 각 패키지가 `repository.GetStatus` 호출 | `pkg/doctor`/`pkg/reposync`는 `*gitcmd.Executor`만 보유 — 의존 역전이 큼 |

결함 2만 국소 수정(`AA`/`DD` 추가)하고 통합은 미루는 축소안도 성립한다.

### 5. `internal/parser`는 패키지 전체가 죽은 코드다

06이 `ParseStatus`를 지웠지만 **남은 7개 함수도 프로덕션 호출자가 0건**이다
(`ParseIsClean` `ParseFileList` `ParseBranchInfo` `ParseAheadBehind` `ParseCommitInfo`
`ParseRemoteInfo` `ParseUpstreamInfo`). `pkg/doctor`는 `parseAheadBehind` 사본을 따로
갖고 있고 그쪽이 쓰인다 — 06이 `ParseStatus`에서 본 것과 같은 구도.
안 B를 고르면 이 정리가 선행되어야 한다.

## References

- 선행: `01-changeset-unify-diff-commit-scope.md`, `06-porcelain-parsers-outside-diff-commit.md`
- 관련: `08-conflict-guard-fail-open-on-git-failure.md` (`executor.Run` fail-open 계열)
- 정답 구현: `pkg/repository/porcelain.go`, `pkg/repository/changeset.go` (`isUnmergedCode`)
