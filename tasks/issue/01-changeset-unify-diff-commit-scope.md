# ISSUE: diff/commit 변경집합(change set) 정의 통일

- status: done (2026-08-05)
- priority: P0
- category: repository/bulk
- created_at: 2026-08-05T13:11:13+09:00
- affects: v0.7.0
- findings: `diff-omits-staged-changes`, `commit-preview-undercounts-untracked-dirs`, `porcelain-quoted-paths-never-unquoted`
- blocks: `04-commit-stats-accuracy-numstat.md`

## Background

`diff`와 `commit`은 "무엇이 변경집합인가"에 대한 **공유된 단일 정의가 없고, 각자 `git status --porcelain` 출력을 독립적으로 재파싱한다.**

| 경로 | 파싱 위치 | `??`(untracked) 처리 | 라인 수 근거 |
|------|-----------|---------------------|--------------|
| `BulkDiff` | `pkg/repository/bulk_diff.go:255-297` | 별도 `UntrackedFiles` 슬라이스로 분리, `FilesChanged`에서 **제외** | `git diff` (worktree↔index) |
| `BulkCommit` | `pkg/repository/bulk_commit.go:310-325` | 구분 없이 `ChangedFiles`에 **포함** | `--stat --cached` + `--stat`의 산술 합 |
| `executeCommit` | `pkg/repository/bulk_commit.go:368` | `git add -A` — **무조건 전부 스테이징** | — |

실제 커밋되는 집합의 정의는 오직 **HEAD → worktree(untracked 포함)** 하나인데, **이를 계산하는 코드가 저장소 어디에도 없다.** `git diff HEAD`는 단 한 번도 호출되지 않는다.

또한 `internal/parser/status.go:119`에 이미 porcelain 분류 로직이 존재하지만 두 bulk 경로 모두 이를 쓰지 않고 파싱을 재구현했다.

## Findings covered

| ID | 위치 | 증상 |
|----|------|------|
| (원 보고) 스코프 불일치 | `bulk_diff.go:284` vs `bulk_commit.go:316` | 동일 리포에서 diff `files_changed=4`, commit `files_changed=6` |
| `diff-omits-staged-changes` | `bulk_diff.go:309`, `:332` | 이미 staged된 변경은 파일 목록엔 뜨지만 diff 본문·라인 수가 전부 0 |
| `commit-preview-undercounts-untracked-dirs` | `bulk_commit.go:301` | `--porcelain`에 `-uall`이 없어 `?? docs/` 1건으로 축약, 미리보기와 실제 커밋 파일 수가 다름 |
| `porcelain-quoted-paths-never-unquoted` | `bulk_diff.go:268`, `bulk_commit.go:320` | C-quoted 경로를 따옴표째 방출, 커밋 메시지 scope까지 오염 |

## Reproduction

tracked 4개 수정 + untracked 3개 파일(디렉터리 2개로 축약) 픽스처:

```
$ git status --porcelain
 M tracked1.txt … M tracked4.txt
?? docs/
?? tasks/

$ gz-git diff --format json      → files_changed=4, untracked_files=["docs/","tasks/"]
$ gz-git commit --dry-run --format json → files_changed=6, changed_files=[…,"docs/","tasks/"]
$ git add -A && git commit       → 실제 커밋 7파일
```

staged 누락:

```
$ git status --porcelain     → M  f1.txt / M  f2.txt / A  n1.txt
$ git diff --stat            → (출력 없음)   ← gz-git이 실제로 실행하는 명령
$ git diff HEAD --stat       → 3 files changed, 3 insertions(+)   ← 진실
$ gz-git diff --format json  → files_changed=3, additions/deletions/diff_content 키 자체가 없음
```

quoted path:

```
$ touch "has space.txt"
$ gz-git diff --format json → untracked_files: ["\"has space.txt\""]   ← 디스크에 없는 문자열
실제 히스토리 커밋 제목: chore("dir with space): update 5 files
```

> `core.quotePath` 기본값(`true`)에서는 비ASCII 경로 전체가 8진 이스케이프로 깨진다.
> 현 개발 머신은 `core.quotePath=false`라 한글 경로는 정상이나, **공백 포함 경로는 설정과 무관하게 깨진다**(재현 확인).

## Scope

`pkg/repository/changeset.go` 신규 — `bulk_diff`/`bulk_commit` 공용 수집기:

```go
type ChangeEntry struct {
	Path, OldPath, Status         string // Path: unquoted, dir-expanded, repo-relative 실경로
	Staged, Untracked, Conflicted bool
}

type ChangeSet struct {
	Entries                                   []ChangeEntry
	TrackedCount, UntrackedCount, StagedCount int
	ConflictCount                             int
	Additions, Deletions                      int
	Scope                                     ChangeScope
}

// ScopeHead(기본, `add -A`와 동치) | ScopeStagedOnly | ScopeWorktreeOnly
func collectChangeSet(ctx context.Context, ex CommandExecutor, repoPath string, scope ChangeScope) (*ChangeSet, error)
```

핵심 교체 3가지:

1. `git status --porcelain` → **`git status --porcelain -z -uall`**
   - `-z`가 C-quoting을 원천 제거(`porcelain-quoted-paths-never-unquoted` 해결)
   - `-uall`이 디렉터리 축약 제거(`commit-preview-undercounts-untracked-dirs` 해결)
   - 부수 효과: `strings.TrimSpace(line)` 후 `line[:2]` 슬라이싱 제거 → index/worktree 상태 구분 복원
2. diff 본문은 scope에 따라 `git diff HEAD --unified=N` / `--cached` 선택 (`diff-omits-staged-changes` 해결)
3. `bulk_diff.go:255-295` 인라인 파서와 `bulk_commit.go:310-346`을 이 함수 호출로 대체

### JSON 스키마 하위호환

기존 키(`files_changed`, `additions`, `deletions`, `changed_files`, `untracked_files`, `diff_summary`, `diff_content`, `truncated`)는 **이름·타입 그대로 유지**하고 값만 정확해지게 한다. 축약 엔트리가 실경로로 확장되어 카운트가 늘어나는 것은 의도된 정정이므로 CHANGELOG에 breaking-fix로 명시.

신규 키는 전부 `omitempty`로만 추가:
`tracked_files_changed`, `untracked_files_changed`, `staged_files_changed`, `scope`(`"head"|"staged"|"worktree"`)

## Acceptance Criteria

- [x] `collectChangeSet`가 `bulk_diff`/`bulk_commit` 양쪽에서 호출되고, 인라인 porcelain 파서 2곳이 제거됨
      → `bulk_diff.go` ~95줄, `bulk_commit.go` 파싱 루프 삭제. 두 경로의 자체 porcelain 호출 0건
- [x] 동일 리포에 대해 `gz-git diff`의 파일 집합 == `gz-git commit --dry-run`의 파일 집합 == `git add -A && git show --name-only HEAD`의 집합
      → `TestDiffAndCommitAgreeOnFileSet` (3자 동시 비교), `TestChangeSetMatchesAddAllCommit`
- [x] 전부 staged인 리포에서 `gz-git diff`가 비어있지 않은 `diff_content`와 정확한 `additions`/`deletions`를 반환
      → `TestDiffReportsStagedContent`, `TestChangeSetStagedScopeCountsStagedChanges`
- [x] untracked 디렉터리가 개별 파일 경로로 전개됨 (중첩 깊이 무관)
      → `TestChangeSetExpandsUntrackedDirectories`(중첩 `docs/adr/`), `TestCommitPreviewCountsUntrackedFilesNotDirectories`
- [x] 공백 포함 경로가 따옴표 없이 실경로로 반환됨 (`core.quotePath` 양쪽 값 모두에서)
      → `TestChangeSetUnquotesPaths` — quotePath=true/false 서브테스트, 각 경로를 `os.Lstat`으로 실재 확인
- [x] 기존 JSON 키 이름·타입 불변 — 기존 소비자 파싱 코드 무수정 동작
      → 기존 키 편집 없음. 신규 4키(`scope`, `tracked_files_changed`, `untracked_files_changed`, `staged_files_changed`) 전부 `omitempty`
- [x] 고정 픽스처 테스트 추가: untracked 디렉터리(중첩), 공백 경로, 한글 경로, 전부 staged, 부분 staged
      → `changeSetFixture` 단일 픽스처가 5개 조건 모두 포함. `TestChangeSetPartiallyStaged`가 " M"/"M " 구분 검증

## Decisions

1. ~~**`Additions`/`Deletions`는 tracked 경로만 집계한다.**~~
   → **task 04에서 번복됨.** untracked 라인을 빼면 diff와 commit이 라인 수에서 다시
   어긋나 본 태스크의 전제가 무너진다(`commit-untracked-lines-never-counted`).
   task 04이 `addUntrackedAdditions`로 집계를 추가했다. 우려했던 비용은 `.gitignore`
   대상이 `--porcelain -uall`에 잡히지 않아(= `node_modules` 등 제외) 실질적으로 작고,
   읽기는 32KB 청크 스트리밍이라 메모리 상수다.

2. **numstat 전환을 task 04에서 앞당겼다.**
   `--stat` 산문 파싱은 로컬라이즈되고 파일 목록을 폭 제한으로 생략하며 +/- 를 비례
   막대로 렌더링한다. scope별 diff를 어차피 한 곳에서 실행하게 되므로 `--numstat -z`를
   `collectDiffStats`에 함께 넣는 편이 두 번 고치는 것보다 짧다. task 04에는 카운터
   산술·exit code·죽은 파서 제거만 남는다.

3. **`bulk_commit`의 `--stat` 이중 집계 제거(부수 정정).**
   기존 코드는 `--stat --cached`와 `--stat`의 합을 썼는데, staged 후 worktree에서 다시
   수정된 파일은 양쪽에 모두 잡혀 라인 수가 중복 계상됐다. `git diff HEAD --numstat -z`
   한 번이 정확한 답이다.

4. **`gitcmd.Executor.Run`은 git 실패 시 error를 반환하지 않는다 — `runGit` 도입.**
   `Run`은 실패를 `Result.ExitCode`로 알리고 error는 nil로 준다(프로세스 기동 실패만
   error). `err`만 검사하면 "git 실패"가 "변경 없음"으로 뒤집힌다: 깨진 status는 clean
   리포로, 깨진 `ls-files --unmerged`는 충돌 없는 리포로 보고된다. `changeset.go`의 git
   호출 4곳을 `runGit`으로 통일해 두 신호를 단일 error로 합쳤다.
   → 이 버그로 unborn HEAD 폴백이 죽은 코드였고, `TestChangeSetUnbornHead`가 이를 잡았다.

5. **`listUntrackedFiles`(task 03) 삭제.**
   `--porcelain -z -uall`이 동일한 데이터를 이미 주므로 별도 `ls-files --others` 호출이
   중복이 됐다.

## Follow-up

1. `extractDiffSummaryLine`과 `parseDiffStats`는 이제 프로덕션에서 호출되지 않는다
   (테스트만 참조: `bulk_diff_test.go:433`, `bulk_commit_test.go:427`). 삭제는 task 04에서.

2. **동일 버그 계열이 diff/commit 밖에 2곳 더 남아있다** (본 태스크 범위 밖, 신규 태스크
   `06-porcelain-parsers-outside-diff-commit.md`로 등록):
   - `bulk.go:1813` — `UncommittedFiles = len(lines)`로 축약 디렉터리를 1건으로 계상,
     `line[3:]`로 C-quoted 경로를 그대로 방출, 충돌 판정도 `isUnmergedCode` 대신 재구현
   - `client.go:447` — `parseStatus`가 `-z`/`-uall` 없이 동작

## References

- 감사 결과 전문: workflow `wf_6c7e7604-0aa` (12건 확인, 0건 기각)
- 관련 태스크: `02-commit-merge-conflict-guard.md`, `03-untracked-read-loop-security-and-oom.md`, `04-commit-stats-accuracy-numstat.md`, `05-diff-output-untracked-visibility.md`
