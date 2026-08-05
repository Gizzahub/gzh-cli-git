# ISSUE: 미해결 merge conflict 리포를 그대로 커밋 (비가역 손상)

- status: done (2026-08-05)
- priority: P0
- category: repository/bulk
- created_at: 2026-08-05T13:11:13+09:00
- affects: v0.7.0
- findings: `commit-commits-merge-conflict-markers`
- severity: **12건 중 유일한 비가역(irreversible) 손상**

## Background

`analyzeRepositoryForCommit`(`pkg/repository/bulk_commit.go:333`)은 porcelain 상태 코드 중 `U*`(unmerged) 계열을 전혀 검사하지 않고 `status = "dirty"`로 분류한다. 이어 `executeCommit`(`:368`)이 `git add -A`를 실행하는데, git에서 `add`는 **conflict를 "해결됨"으로 표시하는 동작**이므로 `<<<<<<<` 마커가 든 파일이 그대로 인덱스에 올라간다.

`.git/MERGE_HEAD`가 존재하므로 이어지는 `git commit`은 **부모 2개짜리 merge commit**을 생성하고 `MERGE_HEAD`를 삭제한다. 결과적으로 리포는 `status` 상 clean으로 보고되어 **사후 탐지가 사실상 불가능하다.**

`internal/parser/status.go:119`에 이미 `U` 분류 로직이 있으나 bulk 경로는 이를 사용하지 않는다.

## Findings covered

| ID | 위치 | 증상 |
|----|------|------|
| `commit-commits-merge-conflict-markers` | `bulk_commit.go:333` (분류), `:368` (`add -A`) | conflict 마커가 히스토리에 영구 기록, 리포는 clean으로 보고 |

## Reproduction

직접 재현 확인 (2026-08-05):

```
$ git status --porcelain
UU f.txt
$ test -e .git/MERGE_HEAD → YES

$ gz-git commit --dry-run --format json
{"status": "would-commit", "files_changed": 1,
 "changed_files": ["f.txt"], "suggested_message": "chore: update f.txt"}
   ← conflict를 한 마디도 언급하지 않음

$ gz-git commit --yes
$ test -e .git/MERGE_HEAD → GONE
$ git status --porcelain  → 0 entries (clean으로 보임)
$ git rev-list --parents -n1 HEAD → 3 tokens (merge commit)
$ git show HEAD:f.txt
<<<<<<< HEAD
MAIN
=======
OTHER
>>>>>>> other
```

`--yes`가 붙은 무인 bulk 루프에서 특히 위험하다.

## Scope

1. `collectChangeSet`(`01-changeset-unify-diff-commit-scope.md`)에서 `ConflictCount` / `ChangeEntry.Conflicted` 산출
   - porcelain 상태 코드 `DD`, `AU`, `UD`, `UA`, `DU`, `AA`, `UU` 전부
   - 보강 검사로 `git ls-files -u` 비어있지 않음 또는 `.git/MERGE_HEAD` 존재
2. `BulkCommit`에서 `ConflictCount > 0`인 리포는 **커밋하지 않고** `status: "skipped"` + 명시적 `error` 메시지로 처리
3. `--dry-run` 미리보기에서 conflict 상태를 별도 아이콘/라벨로 표시 (`⚠ conflict` 등)
4. 프로세스 exit code를 0이 아닌 값으로 올림 (현재는 실패해도 0 — `04` 태스크와 공통 이슈)
5. 강제 실행이 필요하다면 `--allow-conflicted` 같은 별도 opt-in 플래그로만 허용

### JSON 스키마

`omitempty` 신규 키 `conflicted_files` (`[]string`) 추가. 기존 키 불변.

## Decisions

**1. `internal/parser/status.go`의 `U` 분류 로직은 재사용하지 않는다.**
해당 코드는 `internal/` 밑이라 `pkg/repository`에서 쓰려면 의존 방향이 `pkg → internal/parser`로 새로
생기고, 반환 타입이 이 경로에 필요 없는 필드를 끌고 온다. 대신 `isUnmergedCode`를
`pkg/repository/changeset.go`에 **8줄로** 두었다 — git이 정의한 unmerged 조합은 정확히 7개
(`DD AU UD UA DU AA UU`)이고 `AA`/`DD`를 제외하면 전부 한쪽이 `U`이므로 두 번의 비교로 끝난다.
파일 위치를 `changeset.go`로 고른 것은 `01` 태스크의 `collectChangeSet`이 같은 파일에 들어와
이 함수를 그대로 흡수하도록 하기 위한 것이다 (재작성 없음).

**2. 탐지원과 경로원을 분리한다.**
- 탐지: porcelain 상태 코드 (이미 파싱 중이므로 추가 비용 0)
- 경로: `git ls-files --unmerged -z` — 인덱스 stage 1~3을 직접 읽으므로 `core.quotePath`의
  영향을 받지 않고 워크트리 현재 상태와도 무관하다. stage당 1레코드라 중복 제거가 필요하다.
- `ls-files`가 빈 결과를 주면 porcelain 경로로 폴백한다. **탐지된 conflict가 무음으로
  강등되는 일은 없다.**

**3. 상태명은 `"skipped"`가 아니라 `"conflicted"`.**
초안은 `skipped`를 제안했으나 그 값은 이미 "변경 없음"에 쓰인다. 거부를 정상 스킵과 같은 이름으로
묶으면 정확히 이 버그가 눈에 띄지 않았던 방식으로 다시 묻힌다. `TotalConflicted`도 별도 카운터로
두고 `TotalDirty`에서 제외했다 — Phase 2에 넘어가지 않는 리포를 dirty로 세면
커밋 성공률이 실패처럼 보인다.

## Acceptance Criteria

- [x] `UU`/`AA`/`DD` 등 unmerged 상태 리포가 `gz-git commit --yes`로 커밋되지 않음
      → `TestBulkCommitRefusesUnmergedRepository`
- [x] `--dry-run` 출력(default/compact/json/llm 전부)에 conflict 상태가 명시적으로 표시됨
      → `TestBulkCommitDryRunReportsConflict`; default/compact는 `⊗ CONFLICT: N unmerged` +
      unmerged 경로 전체를 `--verbose` 무관하게 출력, JSON은 `conflicted_files` / `total_conflicted`
- [x] conflict 리포가 있을 때 프로세스 exit code != 0
      → `errPartialFailure(TotalFailed+TotalConflicted, TotalDirty+TotalConflicted)` = `ExitPartialFailed`(2)
- [x] `.git/MERGE_HEAD`가 보존되어 사용자가 수동으로 merge를 이어갈 수 있음
      → 같은 테스트에서 `MERGE_HEAD` 존재 + `HEAD` 부모 1개를 함께 확인
- [x] `bulk_commit_test.go`에 `UU` 픽스처 테스트 추가
      → `bulk_commit_conflict_test.go` 신규 4건 (`createConflictedRepo` 픽스처)
- [x] `internal/parser/status.go`의 기존 `U` 분류 로직 재사용 여부 검토 후 결정 기록 → 위 Decision 1

## Escape hatch

`--allow-conflicted`로만 강제 커밋 가능. 플래그 설명에
"writes conflict markers into history"를 명시했다.
`TestBulkCommitAllowConflictedOptIn`이 opt-in 시 부모 2개 merge commit이 정상 생성되는지 확인한다.

## References

- 감사 결과 전문: workflow `wf_6c7e7604-0aa`
- 우회법 (수정 전까지): bulk 실행 전 `git -C <repo> ls-files -u | head -1` 및 `test -e <repo>/.git/MERGE_HEAD`로 호출자가 직접 차단
