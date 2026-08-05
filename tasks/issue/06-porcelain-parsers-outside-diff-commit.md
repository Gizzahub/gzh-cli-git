# ISSUE: diff/commit 밖에 동일한 porcelain 파싱 결함이 2곳 더 존재

- status: done (2026-08-05)
- priority: P3
- category: repository
- created_at: 2026-08-05T16:00:00+09:00
- scoped_at: 2026-08-05T18:30:00+09:00
- affects: v0.7.0
- spawned_from: `01-changeset-unify-diff-commit-scope.md` (Follow-up #2)
- spawns: `08-conflict-guard-fail-open-on-git-failure.md`

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

> **같은 블록의 fail-open 결함은 태스크 08로 분리했다** — git이 exit≠0으로 죽어도 에러 없이
> "충돌 없음"으로 판정되어 `push` 가드가 열린다. 심각도 계열이 다르므로(P2) 별도 추적.
> 본 태스크에서 `runGit` 전환을 함께 하게 되면 08의 Scope 1이 자연히 해소되므로,
> **08을 닫기 전에 실제 diff로 확인할 것.**

### 2. `pkg/repository/client.go:447` — `GetStatus` → `parseStatus` (`:501`)

```go
output, err := c.executor.RunOutput(ctx, repo.Path, "status", "--porcelain")  // -z/-uall 없음
...
func parseStatus(output string) (*Status, error) {
    ...
    filePath := strings.TrimSpace(line[3:])                                    // C-quoted 경로
```

- `status --porcelain`을 `-z`/`-uall` 없이 호출. 같은 클래스.
- `line[3:]` → C-quoted 경로 방출. rename 처리 분기가 별도로 있으나(`:533`) 비ASCII는 깨짐.

#### 2-a. `RunOutput`의 TrimSpace가 첫 레코드를 오독시킨다 (v0.7.0 실측 재현)

`RunOutput`은 반환 직전 `strings.TrimSpace(result.Stdout)`를 건다
(`internal/gitcmd/executor.go:211`). `parseStatus`는 주석에

```go
// Don't trim the line itself as git status --porcelain has specific format
```

이라고 명시하며 컬럼 오프셋(`line[0]`, `line[1]`, `line[3:]`)으로 파싱하는데,
**stdout 전체가 이미 트림된 뒤**라 첫 레코드의 선행 공백이 사라진 상태로 들어온다.

```
raw porcelain:   " D a.txt"      (index=' ', worktree='D' → 워크트리 전용 삭제)
TrimSpace 후:    "D a.txt"       (첫 줄만; 둘째 줄부터는 선행 공백 유지)
parseStatus:     index='D'  → StagedFiles += line[3:] == ".txt"
```

실측 (설치된 `gz-git v0.7.0`, 커밋된 `a.txt`/`b.txt`를 워크트리에서 삭제):

```console
$ git status --porcelain
 D a.txt
 D b.txt
$ gz-git status --skip-fetch --format json ./del | grep uncommitted
      "uncommitted_files": 1        # 기대값 0 (modified+staged 합; 둘 다 워크트리 전용 삭제)
```

`1`이 나오는 이유가 정확히 위 오독이다 — 첫 줄만 staged로 오분류되고 경로는 `.txt`로 잘린다.
인덱스 상태가 공백인 코드(` M`, ` D`, ` T`)가 목록 **첫 줄**일 때 항상 발동하며,
"수정했으나 스테이징 안 함"이 가장 흔한 경우라 실사용 빈도가 높다.

영향:

- `pkg/watch/watcher.go:333-350` — 첫 파일에 대해 staged/modified 이벤트 종류가 뒤바뀐다.
- `pkg/reposync/diagnostic_executor.go:170` — `modified+staged` 합산이라 카운트 오차로 나타난다.
- `bulk_switch.go:175`, `update.go:309` — 사용자에게 보이는 파일 수가 틀린다.
- `status.IsClean`은 영향 없음(어느 쪽으로 분류되든 false) → **의사결정 가드는 안전.**

> 이 결함은 06의 처방(`-z -uall` + 공유 파서)으로 `RunOutput` 경유가 없어지면 부수적으로
> 해소된다. 별도 태스크로 쪼개지 않고 여기 흡수한다.

#### 2-b. `internal/parser/status.go:37` 중복은 **죽은 코드**

`internal/parser.ParseStatus`는 `pkg/repository.parseStatus`와 사실상 동일하다(에러 타입만 다름).
**프로덕션 호출자가 0건** — `internal/parser/status_test.go`만 호출한다.
따라서 "중복 제거"는 통합이 아니라 **삭제**로 끝난다.

```console
$ grep -rn "parser\.ParseStatus" --include="*.go" .
(없음)
```

### 3. `UncommittedFiles` 소비자 조사 결과 (착수 전 확인 항목)

`repositoryState`는 패키지 비공개 타입이고, `UncommittedFiles`를 읽는 프로덕션 코드는 1곳뿐이다.

```
bulk.go:2454  result.UncommittedFiles = repoState.UncommittedFiles   (processStatusRepository)
  └→ cmd/bulk_render.go:214,234,257   아이콘/요약 문자열
  └→ cmd/status.go:330, cmd/info.go:158   JSON·표시
```

- `state.IsDirty`(`bulk.go:1833`)는 **대입만 되고 프로덕션 리더가 0건**
  (`bulk_state_test.go:82,98`만 읽음). 사실상 죽은 필드.
- 실제 의사결정 게이트 — `bulk_switch.go:172`(더티 리포 스킵), `update.go:307/404`,
  `bulk.go:1587`(auto-stash) — 는 전부 `GetStatus`의 `status.IsClean`을 쓴다.
  `IsClean`은 불리언이라 `?? docs/` 축약에 둔감하다(축약돼도 false).

**결론: `UncommittedFiles` 축약 결함 자체는 표시 전용이며 P3가 맞다.**

#### 3-a. 부수 발견 — untracked 이중 계상

```go
// bulk.go:2454-2455
result.UncommittedFiles = repoState.UncommittedFiles   // untracked 엔트리 포함
result.UntrackedFiles   = len(status.UntrackedFiles)   // untracked 다시
```

`bulk_render.go:234`가 `"[dirty: %d uncommitted, %d untracked]"`로 둘을 나란히 출력하므로
untracked가 두 번 세어진다. 축약을 풀면 이 오차가 **커진다**.

## Scope (확정 — 2026-08-05)

1. 저수준 파서 `parsePorcelainZ(stdout) → []record{code, path, oldPath}` 를 하나 추출하고
   `collectChangeSet` · `parseStatus` · `checkRepositoryState` 셋이 공유한다.
   - `collectChangeSet` 재사용이 아니라 **공통 저수준 파서**인 이유: `ChangeEntry`는
     `parseGitStatus(code)`로 XY를 한 글자로 접어 인덱스/워크트리 어느 쪽 변경인지를 잃는다.
     `Status`의 `StagedFiles`/`ModifiedFiles`/`DeletedFiles` 구분은 원본 XY 코드를 요구하므로
     `ChangeSet` 경유로는 무손실 복원이 불가능하다.
2. 두 호출을 `--porcelain -z -uall` + `runGit`으로 전환 (`RunOutput` 경유 제거 → 2-a 해소).
3. `checkRepositoryState`의 충돌 판정을 `isUnmergedCode` 호출로 대체하고,
   `ConflictedFiles`는 `collectConflictedPaths`(`changeset.go:458`, `ls-files --unmerged -z`)에서 받는다.
4. `internal/parser/status.go`의 `ParseStatus`/`parseStatusCode` **삭제** (호출자 0건).
   `internal/parser/status_test.go`의 해당 케이스도 함께 제거.
5. `UncommittedFiles` 의미 재확정 — **deprecate + 세분화** (결정됨):
   - `repositoryState.UncommittedFiles` / `RepositoryStatusResult.UncommittedFiles`를
     `Deprecated:` 주석으로 표시하고 값은 유지(축약만 해제).
   - 세분화 카운트를 신설해 노출: 추적 변경 / 스테이징 / untracked를 분리.
   - `repositoryState.IsDirty`는 리더가 없으므로 함께 제거 검토.
   - 소비자 갱신 대상: `cmd/bulk_render.go`, `cmd/status.go`, `cmd/info.go`,
     `cmd/pull.go`, `cmd/fetch.go`, `cmd/push.go`, `cmd/bulk_common.go`.

### 하위호환

`Status`/`repositoryState`의 공개 필드값이 바뀐다:

- 축약 해제로 카운트가 **증가**하는 방향 (`?? docs/` 1 → 실제 파일 수 N)
- 3-a의 이중 계상을 정리하면 표시 값이 **감소**하는 방향
- 2-a 수정으로 첫 레코드가 `StagedFiles` → `ModifiedFiles`/`DeletedFiles`로 **이동**

공개 API이므로 CHANGELOG에 breaking-fix로 명시. 세 방향 모두 개별 항목으로 적을 것
(값이 커지는 것만 적으면 감소 케이스를 만난 사용자가 회귀로 오인한다).

## Acceptance Criteria

- [x] `checkRepositoryState`가 `-uall` 없는 porcelain을 직접 파싱하지 않음
- [x] `parseStatus`가 `-z`/`-uall` 기반 파싱을 사용하거나 단일 파서로 통합됨
- [x] 충돌 판정이 `isUnmergedCode`를 재사용 (중복 분기 제거)
- [x] `internal/parser.ParseStatus` 삭제, 빌드/테스트 통과
- [x] `?? 디렉터리` 축약 케이스에 대한 회귀 테스트 추가
      (`TestGetStatusExpandsUntrackedDirectories` — 실제 git 리포 사용. 테이블 테스트로는
      플래그 전달 여부를 증명할 수 없어 별도 실-리포 테스트로 분리)
- [x] C-quoted/비ASCII 경로가 실경로로 반환됨 (01의 `TestChangeSetUnquotesPaths`와 동일 기준)
- [x] **첫 레코드가 ` D`/` M`일 때 `ModifiedFiles`/`DeletedFiles`로 분류되고 경로가 온전한지** 회귀 테스트
      (`TestParseStatus/unstaged_modify_as_first_record`, `.../worktree-only_delete_as_first_record`)
- [x] `UncommittedFiles` deprecate 주석 + 세분화 필드 추가, 소비자 파일 갱신
- [x] CHANGELOG에 증가/감소/이동 세 방향 모두 breaking-fix로 기재

## 구현 결과 (2026-08-05)

### 변경

| 파일 | 내용 |
|------|------|
| `pkg/repository/porcelain.go` (신규) | `parsePorcelainZ` + `statusFromRecords` + `applyStatusCode` — 패키지 단일 파서 |
| `pkg/repository/changeset.go` | `collectChangeSet`의 인라인 레코드 루프를 `parsePorcelainZ`로 교체 |
| `pkg/repository/client.go` | `GetStatus`가 `runGit` + `-z -uall`; 구 `parseStatus`/`parseStatusCode` 삭제 |
| `pkg/repository/bulk.go` | `checkRepositoryState` 재작성; 4개 결과 구조체에 `TrackedChangedFiles`/`StagedFiles`/`UnstagedFiles` 추가, `UncommittedFiles` deprecate |
| `pkg/repository/interfaces.go` | `Status`에 `StagedCount`/`UnstagedCount`/`TrackedChangedCount` 추가 |
| `internal/parser/status.go` | `ParseStatus`/`parseStatusCode` 삭제 (+ 해당 테스트) |
| `pkg/reposync/diagnostic_executor.go` | `ModifiedFiles` 합산과 dirty 판정을 `TrackedChangedCount`로 |
| `pkg/repository/update.go`, `bulk_switch.go` | 같은 계열의 표시용 합산 3곳 정리 |
| `cmd/gz-git/cmd/{bulk_common,pull,fetch,push,info}.go` | deprecated 필드 대신 `TrackedChangedFiles` 사용 |

### 설계 판단 3건

1. **`repositoryState`에서 카운트 필드를 없앴다.** `UncommittedFiles`를 세분화 필드로
   교체하면 유일한 호출자(`processStatusRepository`)가 이미 `*Status`를 들고 있으므로
   새 필드도 리더 0건이 된다 — 방금 제거한 `IsDirty`와 같은 모양. 카운트는 XY 코드를
   본 유일한 값인 `Status`가 갖고, 이 타입은 "지금 어떤 작업 중인가"만 답한다.
2. **카운트를 `Status`의 필드로 저장한다** (슬라이스에서 파생 불가). `D `(인덱스 삭제)와
   ` D`(워크트리 삭제)가 둘 다 `DeletedFiles`에 들어가므로, 슬라이스 조합의 `len()`으로는
   staged/unstaged를 구분할 수 없고 두 슬라이스에 걸친 경로를 이중 계상한다.
3. **JSON 키 `uncommitted_files`는 유지.** 값만 정정. 키 변경은 값 변경보다 큰 파괴이고,
   이제 네 경로 모두 같은 의미를 갖는다.

### 부수 해소

- **2-a (`RunOutput` TrimSpace 첫 레코드 오독)** — `runGit` 전환으로 해소.
- **3-a (untracked 이중 계상)** — `checkRepositoryState`에서 카운트를 걷어내 해소.
- **`T`(typechange) 하드 실패** — 구 `parseStatusCode`는 `T`가 `default`에 걸려 파싱 전체를
  실패시켰다. 신규 파서는 staged/unstaged로 정상 분류 (`TestParseStatus/typechange`).
- **태스크 08 Scope 1** — `checkRepositoryState`의 fail-open이 `runGit` 전환으로 함께
  해소되고 회귀 테스트(`TestCheckRepositoryStateFailsOnBrokenGit`)를 붙였다.
  **08은 `diagnostic_executor.go:166-168`의 형제 fail-open이 남아 열려 있다.**

### 검증

```console
$ go build ./... && go vet ./... && go test ./...     # 전부 통과
$ gofumpt -l pkg/ cmd/ internal/                      # 출력 없음
```

golangci-lint은 실행 불가 — 설치본이 go1.25로 빌드되어 있고 모듈이 go1.26을 타깃한다
(기존 환경 문제, 이 변경과 무관).

실측 대조 (`uncommitted` + `untracked`, git ground truth 대비):

| 픽스처 | git 실제 | v0.7.0 | 신규 |
|--------|---------|--------|------|
| 이 리포 (`info`) | 18 tracked + 3 untracked | 21 + 3 | **18 + 3** |
| 워크트리 삭제 2 + untracked 디렉터리 1개(파일 2) (`info`) | 2 + 2 | 3 + 1 | **2 + 2** |
| 같은 픽스처 (`status --skip-fetch`, 헬스 경로) | 2 + 2 | 1 + 1 | **2 + 2** |

## References

- 원본 감사: workflow `wf_6c7e7604-0aa`
- 관련 태스크: `01-changeset-unify-diff-commit-scope.md` (동일 결함 계열, 커밋/표시 경로)
- 파생 태스크: `08-conflict-guard-fail-open-on-git-failure.md` (같은 함수, fail-open 계열 P2)
