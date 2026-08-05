# ISSUE: commit의 additions/deletions 집계 부정확 + 실패가 exit code에 안 드러남

- status: done (2026-08-05)
- priority: P2
- category: repository/bulk
- created_at: 2026-08-05T13:11:13+09:00
- affects: v0.7.0
- findings: `commit-stats-sum-not-head-delta`, `commit-additions-double-count-staged-plus-unstaged`, `commit-untracked-lines-never-counted`, `parse-diff-stats-filename-poisoning`
- depends_on: `01-changeset-unify-diff-commit-scope.md`

## Background

`analyzeRepositoryForCommit`(`pkg/repository/bulk_commit.go:336-346`)은 라인 수를 두 번의 `--stat` 호출을 **산술 합산**해 구한다.

```go
diffResult, _ := c.executor.Run(ctx, repoPath, "diff", "--stat", "--cached")
result.Additions, result.Deletions = parseDiffStats(diffResult.Stdout)

diffUnstagedResult, _ := c.executor.Run(ctx, repoPath, "diff", "--stat")
additions, deletions := parseDiffStats(diffUnstagedResult.Stdout)
result.Additions += additions      // ← 겹치는 라인이 2번 계상
result.Deletions += deletions
```

`git diff HEAD`(실제 커밋될 순변화)는 호출하지 않으므로, 두 diff가 겹치면 과대, 상쇄되면 완전히 틀린다. untracked 파일은 어느 쪽 `--stat`에도 잡히지 않는다.

추가로 `parseDiffStats`(`:478`)의 라인 필터가 부정확하다.

```go
if strings.Contains(line, "changed") {   // ← 파일명 라인까지 매칭
```

`git diff --stat` 출력에는 요약 라인뿐 아니라 **파일명 라인**도 포함되므로, 파일명에 `changed`가 들어있으면 그 라인의 선두 숫자가 삽입/삭제 수로 둔갑한다.

## Findings covered

| ID | 위치 | 증상 |
|----|------|------|
| `commit-stats-sum-not-head-delta` | `:336`+`:342` | 순변화 0인 리포를 `would-commit`으로 예고 → 실행 시 결정적 실패, **exit code는 0** |
| `commit-additions-double-count-staged-plus-unstaged` | `:342` | 겹치는 편집이 정확히 2배로 계상 |
| `commit-untracked-lines-never-counted` | `:336-346` | untracked는 `files_changed`엔 잡히나 `additions`엔 절대 안 잡힘 |
| `parse-diff-stats-filename-poisoning` | `:478` | 파일명 선두 숫자가 삽입/삭제 수로 둔갑 |

## Reproduction

**상쇄 케이스** (`MM f1.txt` — staged 후 worktree를 되돌림):

```
$ gz-git commit --dry-run  → would-commit, +2/-2
$ git diff HEAD --stat     → (출력 없음)  ← 실제 순변화는 0
$ gz-git commit --yes      → status:"error", "nothing to commit, working tree clean"
$ echo $?                  → 0            ← 실패인데 성공 종료코드
```

**이중 계상** (같은 줄을 stage 후 재편집):

```
실제 git diff --stat HEAD → 1 insertion, 1 deletion
커밋 결과                  → 1 insertion, 1 deletion
gz-git 보고                → additions:2, deletions:2
```

**untracked 미집계**: 신규 파일만 있는 리포 → dry-run에 `additions` 키 없음(0), 실제 커밋은 `1 file changed, 5 insertions(+)`. 혼합 케이스는 보고 2 → 실제 12.

**파일명 오염**:

```
9-changed-deletions.txt 에 3줄 추가 (순수 add)
  git numstat → 3 0
  gz-git      → deletions:9      ← 파일명의 "9"
40-unchanged-insertions.md → additions:40
디렉터리 접두 숫자(10-docs/)도 동일
```

## Scope

1. `parseDiffStats`의 텍스트 파싱을 **폐기**하고 `git diff --numstat -z HEAD`로 교체
   - `--numstat`은 `<add>\t<del>\t<path>` 고정 포맷 → 파일명 오염 원천 차단
   - `-z`는 경로 quoting 제거 (`01` 태스크와 동일 근거)
   - 단일 호출이므로 이중 계상·상쇄 문제 동시 해결
2. untracked 파일의 라인 수를 별도 산출해 합산 (또는 `03` 태스크의 `git add -N` 방식 채택 시 자동 포함)
3. 집계는 `01`의 `collectChangeSet`가 `ChangeSet.Additions/Deletions`로 제공하고, `bulk_commit`은 이를 소비만 한다
4. **exit code 정정**: `TotalFailed > 0`이면 프로세스가 0이 아닌 값 반환
   - 상위 오케스트레이터가 종료코드만 보고 실패한 커밋을 성공으로 집계하는 것을 막는다
   - `02` 태스크(conflict skip)와 동일한 종료코드 정책을 공유

### 함께 확인할 것

`BulkCommit`의 `TotalSkipped = TotalScanned - TotalDirty`(`bulk_commit.go:267`)는 `TotalScanned`가 include/exclude 필터링 **이전** 값이라 `--include`/`--exclude` 사용 시 부풀려진다. 같은 수정 범위에서 정정.

## Acceptance Criteria

- [x] `parseDiffStats` 제거 또는 numstat 파서로 대체 — `git diff --stat` 텍스트 파싱 호출 0건
      → `parseDiffStats`, `extractDiffSummaryLine` 및 각 테스트 삭제. `--stat` 호출 0건
        (요약 문자열은 `formatDiffSummary`가 numstat 수치로 직접 조립)
- [x] 파일명에 `changed`/선두 숫자가 있어도 집계가 정확 (회귀 테스트 픽스처 포함)
      → `TestCommitStatsIgnoreFilenameDigits`(`9-changed-deletions.txt` +3줄 → +3/-0),
        `TestParseNumstat/numeric_filenames`
- [x] staged+unstaged가 겹치는 편집에서 보고값 == 실제 커밋 결과
      → `TestCommitStatsMatchRecordedCommit` — 실제 커밋 후 `git diff HEAD~1 HEAD --numstat`와 대조
- [x] 순변화 0인 리포는 `would-commit`이 아니라 `clean`/`skipped`로 예고됨
      → `TestCommitNetZeroIsNotPreviewedAsCommittable` (`MM` 상쇄 케이스)
- [x] untracked 전용 리포의 `additions`가 실제 커밋 라인 수와 일치
      → `TestCommitCountsUntrackedLines` (개행 없는 마지막 줄 포함, 8줄 일치)
- [x] 커밋 실패가 1건이라도 있으면 exit code != 0
      → task 02에서 이미 반영: `commit.go:273` `errPartialFailure(TotalFailed+TotalConflicted, …)`
        → `cliutil.ExitPartialFailed`(2). 본 태스크에서 경로 재확인만 수행
- [x] `--include`/`--exclude` 사용 시 `TotalSkipped`가 필터링 후 기준으로 정확
      → `TestBulkCommitSkippedCountsFilteredSet/skipped_excludes_filtered-out_repositories`
- [x] 픽스처 추가: `MM` 상쇄, 겹치는 편집, untracked-only, 숫자 접두 파일명
      → `bulk_commit_stats_test.go` 6개 테스트. 바이너리 케이스 추가

## Decisions

1. **`DiffFileCount`(numstat 레코드 수)로 "커밋할 것 없음"을 판정한다 — 라인 수로는 안 된다.**
   바이너리 편집은 `-`/`-`, 모드 변경은 `0`/`0`을 보고하므로 둘 다 순라인 0이지만 실제
   변경이다. `Additions==0 && Deletions==0`으로 게이트하면 이 둘을 조용히 건너뛴다.
   레코드 수만이 "diff 없음"과 "순라인 0인 diff"를 구분한다.
   → `TestCommitCountsBinaryAsZeroLines`가 이 구분을 고정한다.

2. **충돌 리포는 net-zero 판정에서 제외한다.**
   unmerged 경로는 해소 전까지 HEAD 델타가 없어 clean으로 오판될 수 있다. task 02의
   충돌 가드가 먼저 걸리도록 `ConflictCount == 0` 조건을 net-zero 판정에 추가했다.

3. **untracked 라인 집계는 `collectChangeSet`에 넣어 diff/commit이 공유한다.**
   commit 쪽에만 넣으면 같은 리포에서 diff는 `additions=0`, commit은 `additions=12`를
   보고하게 되어 task 01이 없앤 바로 그 불일치가 라인 수 차원에서 부활한다.
   task 03에서 기각한 `git add -N`은 여기서도 채택하지 않았다(같은 이유: diff는 read-only).

4. **`TotalSkipped`는 필터링 **후** 집합 기준(`len(filteredRepos)`)이며 dry-run 반환 이전에 계산한다.**
   두 개의 독립 버그였다 — (a) `TotalScanned`는 필터링 전 값이라 `--include` 사용 시
   제외된 리포까지 "건너뜀"으로 계상됐고, (b) 계산이 dry-run 조기 반환 **뒤**에 있어
   dry-run은 항상 0을 보고했다.

## References

- 감사 결과 전문: workflow `wf_6c7e7604-0aa`
- 우회법 (수정 전까지): `additions`/`deletions`/`files_changed`를 신뢰하지 말고 `git diff --cached --numstat`로 직접 구할 것. `suggested_message`는 초안 용도로만 사용. exit code 대신 `--format json`의 `summary.error`와 각 `repositories[].status`를 파싱할 것
