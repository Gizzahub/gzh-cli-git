# ISSUE: default/compact 포맷에 untracked 신호가 전혀 없음

- status: done (2026-08-05)
- priority: P2
- category: cmd/gz-git
- created_at: 2026-08-05T13:11:13+09:00
- affects: v0.7.0
- related: `01-changeset-unify-diff-commit-scope.md`

## Background

`displayDiffResults`(`cmd/gz-git/cmd/diff.go:194`)의 기본(비-verbose) 출력은 `repo.FilesChanged`만 인쇄한다. 이 값은 구조상 tracked 전용이므로(`bulk_diff.go:284`), **untracked 파일이 존재한다는 사실 자체가 출력에 나타나지 않는다.**

| 포맷 | untracked 표시 | 세부 |
|------|---------------|------|
| default | ✗ 없음 | `%d files`만 인쇄 (`diff.go:194`) |
| compact | ✗ 없음 | `displayDiffResultsCompact`도 `FilesChanged`만 (`diff.go:263`) |
| verbose | ✓ | `displayDiffRepositoryResult`의 "Untracked files:" 섹션 (`diff.go:302`) |
| json / llm | ✓ | `untracked_files` 키 (`diff.go:379`) |

`gz-git commit`이 `git add -A`로 이들을 전부 커밋하므로, 기본 포맷 사용자는 **커밋될 파일의 존재를 알 방법이 없다.**

> 참고: `01` 태스크의 `-uall` 수정 이전에는 json/llm에서도 `docs/`, `tasks/`처럼 **디렉터리 축약 상태**로만 보인다. 즉 포맷을 바꿔도 개별 파일은 드러나지 않는다. 두 태스크가 함께 처리되어야 실효가 있다.

## Reproduction

직접 재현 확인 (2026-08-05). tracked 4개 수정 + untracked 3파일(디렉터리 2개로 축약):

```
$ gz-git diff              # default
  ⚠ repoA  (master)  4 files  +4/-4  4 files changed, 4 insertions(+), 4 deletions(-)
                     ↑ untracked 3파일에 대한 언급 전무

$ gz-git diff --format compact
  ⚠ repoA  master  4  +4/-4  4 files changed, 4 insertio...
                     ↑ 동일

$ gz-git diff --format llm
  UNTRACKED_FILES: docs/ | tasks/      ← 존재는 보이나 개별 파일은 안 보임
```

## 실무 영향

LLM 에이전트에게 `gz-git diff` 출력을 커밋 메시지 근거로 넘기는 워크플로우(`claude-plugin/skills/gz-git/SKILL.md:71`이 권장)에서, 에이전트가 보는 증거와 실제 히스토리에 들어가는 내용이 체계적으로 불일치한다. exit code 0, stderr 공백이라 **손실을 탐지할 신호가 전혀 없다.**

실제 사례: ADR 문서 2건과 태스크 파일 10건이 커밋 메시지 근거에서 통째로 빠져, 실제 커밋 내용과 어긋나는 메시지가 생성될 뻔했다.

## Scope

1. default 포맷의 리포 라인에 untracked 개수를 병기
   - 예: `4 files (+3 untracked)` 또는 `⚠ repoA … 4+3 files`
   - `01` 태스크에서 `tracked_files_changed` / `untracked_files_changed`가 분리되면 그 값을 그대로 소비
2. compact 포맷 테이블에 untracked 컬럼 추가 (또는 Files 컬럼을 `4/+3` 형태로)
3. `03` 태스크의 `omitted_files`가 비어있지 않으면 사람이 읽는 포맷에도 경고 라인 출력
4. `--no-content` 등 기존 플래그와의 상호작용 확인 — 요약 정보는 `--no-content`에서도 유지되어야 함

### 하위호환

출력 컬럼이 늘어나므로 default/compact 출력을 스크립트로 파싱하는 소비자가 있을 수 있다. 다만 이 두 포맷은 애초에 사람용이며 스크립팅용으로는 `--format json`을 안내하고 있으므로(`diff.go:43-44` help 텍스트), 변경 가능하다고 판단. CHANGELOG에 명시.

## Acceptance Criteria

- [x] `gz-git diff` default 출력에서 untracked 파일 존재 여부와 개수를 확인할 수 있음
      → `formatDiffFileCount`가 `4 files (+3 untracked)` 형태로 병기.
        `TestDiffDefaultFormatShowsUntracked`
- [x] `--format compact` 출력에서 동일하게 확인 가능
      → `Untracked` 컬럼 추가 + `Files` → `Tracked` 개명.
        `TestDiffCompactFormatShowsUntrackedColumn`
- [x] untracked가 0건일 때는 불필요한 노이즈가 추가되지 않음
      → default는 접미사 자체를 생략, compact는 컬럼을 만들지 않음(`anyUntracked`).
        `TestDiffDefaultFormatQuietWhenNothingUntracked`,
        `TestDiffCompactFormatKeepsOldShapeWhenNothingUntracked`
- [x] `01` 수정 후 개별 파일 경로가 verbose/json/llm에서 디렉터리 축약 없이 노출됨
      → `TestDiffJSONFormatExposesIndividualPaths`(`/` 접미 경로 0건 단언),
        `TestDiffLLMFormatShowsUntracked`. 실 리포 검증에서 `??` 10건 → 22 파일 전개 확인
- [x] 4개 포맷(default/compact/json/llm) 전부에 대한 골든 출력 테스트 추가
      → `cmd/gz-git/cmd/testdata/*.golden` 8건 + `-update-golden` 재생성 플래그.
        `--no-content` 상호작용까지 `TestDiffVerboseNoContentKeepsSummary`로 고정

## Decisions

1. **compact의 `Untracked` 컬럼은 조건부로만 렌더링한다.**
   0으로 채워진 컬럼은 노이즈이므로 `anyUntracked(result)`가 참일 때만 만든다. 같은
   조건에서 `Files` 헤더를 `Tracked`로 바꾼다 — 파일 개수 컬럼이 둘이 되는 바로 그
   순간에만 `Files`가 모호해지기 때문이다.

2. **`omitted_files` 경고는 3개 사람용 포맷 모두에 넣되, 사유는 verbose에만 남긴다.**
   default는 `⚠ N untracked file omitted from the diff body (--verbose for reasons)`,
   compact는 summary 뒤 `[N omitted]`. 문구는 `omittedNote` 하나에서 나온다.

3. **diff의 `files_changed`는 tracked 전용으로 유지한다(하위호환).**
   commit의 `files_changed`는 총계(28), diff는 tracked(6)로 여전히 다르다. 이는 task 01의
   JSON 하위호환 결정에 따른 **의도된** 차이이며, 두 명령 모두 `tracked_files_changed` /
   `untracked_files_changed`를 함께 실어 소비자가 대조할 수 있게 했다. 사람용 출력에서는
   `6 files (+22 untracked)`로 합이 드러나므로 원 증상(4 vs 7)은 해소된다.

4. **골든은 `-update-golden`으로 생성한다.**
   `%-40s` 패딩을 손으로 옮겨 적으면 코드가 아니라 필사를 테스트하게 된다. 단, 생성만으로는
   검증이 아니다 — 최초 생성된 llm 골든은 임의의 맵 순서 하나를 포착해 통과했고,
   `-count=20` 재실행에서 3회 실패했다(아래 Follow-up 2).

## 실 리포 검증 (2026-08-05)

`gzh-cli-gitforge` 작업 트리 자체(tracked 6 수정 + `??` 10건이 실제 22파일)로 확인:

```
$ gz-git diff                → ⚠ gzh-cli-gitforge (master)  6 files (+22 untracked)  +3697/-297
$ gz-git diff --format compact → Tracked 6 | Untracked 22
$ gz-git commit --dry-run --format json
    → files_changed 28, tracked 6, untracked 22, additions 3697, deletions 297
$ git add -A --dry-run | wc -l → 28        ← 6 + 22
```

## Follow-up

1. default/compact 출력 컬럼 변경은 CHANGELOG에 breaking-fix로 명시 필요 (본 태스크 범위 밖).

2. **LLM 포맷의 맵 출력 순서가 비결정적이다** — `gzh-cli-core/cli/llm_formatter.go:177`이
   `reflect.Value.MapRange`로 정렬 없이 순회하므로, `SUMMARY:`처럼 키가 2개 이상인 맵은
   실행할 때마다 순서가 바뀐다. 기계 소비를 전제한 포맷에서 실제 결함이지만 **다른
   저장소** 소유라 본 태스크에서 고치지 않았다. 골든은 `sortLLMSummaryBlock`으로 정규화해
   임의의 한 순열을 정답으로 굳히는 것을 피했다.
   → `07-llm-output-nondeterministic-map-order.md`

## References

- 감사 결과 전문: workflow `wf_6c7e7604-0aa`
- 우회법 (수정 전까지): `git add -A` 후 `gz-git diff --staged` 사용 — untracked 전개·경로 처리·본문을 전부 git이 담당
