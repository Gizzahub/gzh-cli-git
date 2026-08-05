# Tasks — gzh-cli-gitforge

> Last Updated: 2026-08-05

## 구조

```
tasks/
├── issue/    # 확인된 결함 / 후속 설계 필요 항목
├── todo/     # 착수 대기 (범위 확정됨)
├── doing/    # 진행 중
└── done/     # 완료
```

규약: **1파일 = 1작업**, `NN-kebab-case-title.md`, 인덱스 파일은 `README.md`/`INDEX.md`만.
(`gzh-cli/tasks/` 규약 준용)

---

## 완료 (2026-08-05)

| # | 태스크 | 우선순위 | 요지 |
|---|--------|---------|------|
| 01 | [changeset-unify-diff-commit-scope](issue/01-changeset-unify-diff-commit-scope.md) | **P0** | `diff`/`commit`이 변경집합 정의를 공유하지 않음 — 공용 `collectChangeSet` 도입 ✅ |
| 02 | [commit-merge-conflict-guard](issue/02-commit-merge-conflict-guard.md) | **P0** | 미해결 merge conflict를 그대로 커밋 (유일한 비가역 손상) ✅ |
| 03 | [untracked-read-loop-security-and-oom](issue/03-untracked-read-loop-security-and-oom.md) | **P0** | `--include-untracked`의 `os.ReadFile` 루프 — 정보유출·OOM·무음 누락 ✅ |
| 04 | [commit-stats-accuracy-numstat](issue/04-commit-stats-accuracy-numstat.md) | P2 | `additions`/`deletions` 부정확, 실패가 exit code에 안 드러남 ✅ |
| 05 | [diff-output-untracked-visibility](issue/05-diff-output-untracked-visibility.md) | P2 | default/compact 포맷에 untracked 신호 전무 ✅ |

**검증**: `go build`·`go vet`·`gofumpt`·`golangci-lint`(신규 결함 0)·전체 테스트 통과.
실 `gzh-cli-gitforge` 작업 트리에서 diff(6+22)/commit--dry-run(28)/`git add -A`(28) 3자 일치 확인.

### 의존 관계 (실행 완료)

```
01 (change-set 통일) ──┬── 04 (stats 정확도)    ✅
                       └── 05 (출력 가시성)     ✅
02 (conflict guard)  ── 독립                   ✅
03 (untracked 읽기)  ── 독립                   ✅
```

**착수 순서**(02 → 03 → 01 → 04 → 05)대로 전부 완료.

---

## Open Issues (후속 — 구현 미착수)

| # | 태스크 | 우선순위 | 요지 |
|---|--------|---------|------|
| 06 | [porcelain-parsers-outside-diff-commit](issue/06-porcelain-parsers-outside-diff-commit.md) | P3 | 동일 porcelain 결함이 `bulk.go:1813`·`client.go:447`에 잔존 (상태/헬스 경로, 비가역 아님) |
| 07 | [llm-output-nondeterministic-map-order](issue/07-llm-output-nondeterministic-map-order.md) | P3 | `gzh-cli-core` LLM 포맷이 맵 키를 정렬 없이 방출 — cross-repo, 회귀 테스트용 정규화로 우회 중 |

---

## 감사 이력

### 2026-08-05 — `diff`/`commit` 변경집합 일관성 감사

발단: `gz-git diff`(4파일)와 `gz-git commit --dry-run`(7파일)의 보고 범위가 달라, LLM 에이전트에 넘길 커밋 메시지 근거에서 ADR 2건과 태스크 파일 10건이 누락됨.

다중 에이전트 감사(17 agents, 4 lens × 적대적 검증) 결과 **12건 확인, 0건 기각**. 이 중 7건은 수동 재현으로 독립 확인.

#### Finding → 태스크 매핑

| Finding ID | 위치 | 태스크 | 수동재현 |
|-----------|------|--------|:-------:|
| (원 보고) 스코프 불일치 | `bulk_diff.go:284` vs `bulk_commit.go:316` | 01 | ✅ |
| `diff-omits-staged-changes` | `bulk_diff.go:309`, `:332` | 01 | |
| `commit-preview-undercounts-untracked-dirs` | `bulk_commit.go:301` | 01 | ✅ |
| `porcelain-quoted-paths-never-unquoted` | `bulk_diff.go:268`, `bulk_commit.go:320` | 01 | ✅ |
| `commit-commits-merge-conflict-markers` | `bulk_commit.go:333`, `:368` | 02 | ✅ |
| `untracked-symlink-dereference-leak` | `bulk_diff.go:349` | 03 | ✅ |
| `include-untracked-noop-on-directories` | `bulk_diff.go:349` | 03 | ✅ |
| `include-untracked-silently-drops-files` | `bulk_diff.go:349` | 03 | |
| `untracked-read-unbounded-memory` | `bulk_diff.go:349` | 03 | |
| `commit-stats-sum-not-head-delta` | `bulk_commit.go:336`, `:342` | 04 | |
| `commit-additions-double-count-staged-plus-unstaged` | `bulk_commit.go:342` | 04 | |
| `commit-untracked-lines-never-counted` | `bulk_commit.go:336-346` | 04 | |
| `parse-diff-stats-filename-poisoning` | `bulk_commit.go:478` | 04 | |
| default/compact 포맷 untracked 미표시 | `diff.go:194`, `:263` | 05 | ✅ |

> 12건이 5개 태스크로 묶인 이유: 여러 finding이 **동일한 수정 지점**을 공유한다.
> `include-untracked-*` 4건은 `bulk_diff.go:343-367` 한 블록, `commit-stats-*` 4건은
> `bulk_commit.go:336-346`의 합산 로직 하나에서 파생된다. 태스크는 "고칠 단위"로 나눴다.

#### 근본 원인

`diff`와 `commit`이 `git status --porcelain`을 **각자 재파싱**하며, `??` 라인 처리와 라인 수 산출 근거가 서로 다르다. 그런데 최종 실행자 `executeCommit`은 `git add -A`를 돌린다. 실제 변경집합의 정의는 **HEAD → worktree(untracked 포함)** 하나인데, **이를 계산하는 코드가 저장소에 없다** — `git diff HEAD`는 한 번도 호출되지 않는다.

`internal/parser/status.go:119`에 이미 porcelain 분류 로직(conflict `U` 포함)이 존재하나 두 bulk 경로 모두 이를 쓰지 않고 재구현했다. 이 중복이 문제의 표면적 징후다.

---

## v0.7.0 사용자 우회법 (수정 전까지)

핵심: **스캔 전에 스테이징해서 변경집합 계산을 git에게 넘긴다.**

```bash
# 1) conflict 사전 차단 (gz-git은 검사하지 않음)
git -C <repo> ls-files -u | head -1        # 비어있지 않으면 제외
test -e <repo>/.git/MERGE_HEAD && exit 1

# 2) 스테이징 후 --staged 로 증거 수집 (executeCommit이 어차피 하는 일과 동일)
git -C <repo> add -A
gz-git diff <dir> --staged --max-size 500

# 3) 커밋 후 대조
git -C <repo> show --name-only HEAD
```

- `--include-untracked`는 **쓰지 말 것** — 태스크 03의 4개 결함 전부가 이 플래그 경로에서만 발생한다.
- `files_changed`/`additions`/`deletions`는 신뢰하지 말고 `git diff --cached --numstat`로 직접 구할 것.
- 커밋 실패 시에도 exit code가 0이므로, `--format json`의 `summary.error`와 `repositories[].status`를 반드시 파싱할 것.
