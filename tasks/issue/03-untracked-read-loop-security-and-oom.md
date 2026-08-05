# ISSUE: --include-untracked의 os.ReadFile 루프 — 정보유출 · OOM · 무음 누락

- status: done (2026-08-05)
- priority: P0
- category: repository/bulk
- created_at: 2026-08-05T13:11:13+09:00
- affects: v0.7.0
- findings: `untracked-symlink-dereference-leak`, `include-untracked-noop-on-directories`, `include-untracked-silently-drops-files`, `untracked-read-unbounded-memory`
- 공통 원인: `pkg/repository/bulk_diff.go:343-367` 단일 블록

## Background

`getRepositoryDiff`의 `IncludeUntracked` 블록은 untracked 파일 본문을 git이 아니라 **직접 `os.ReadFile`로 읽어 합성 diff hunk를 만든다.** 이 한 블록에서 네 가지 독립적인 결함이 파생된다.

```go
// bulk_diff.go:343-367 (요약)
for _, file := range result.UntrackedFiles {
    content, err := os.ReadFile(filepath.Join(repoPath, file))
    if err != nil { continue }          // ← 무음 누락 (#2 #3)
    ...                                  // ← symlink 추적 (#1)
    if len(result.DiffContent)+len(untrackedDiff) > opts.MaxDiffSize {  // ← 읽은 뒤 검사 (#4)
```

핵심은 `os.ReadFile`가 (a) symlink를 따라가고, (b) 디렉터리에서 EISDIR로 실패하며, (c) 크기 제한 **이전에** 전체를 메모리에 올린다는 점이다. git 자신은 이 세 가지를 모두 올바르게 처리한다.

## Findings covered

| ID | 증상 | 심각도 |
|----|------|--------|
| `untracked-symlink-dereference-leak` | symlink를 따라가 **리포 밖 파일 본문**을 diff/LLM 출력에 인라인 | 🔴 정보유출 |
| `untracked-read-unbounded-memory` | `MaxDiffSize` 검사가 파일을 다 읽은 **후** 수행되어 예산이 무력화 | 🔴 OOM |
| `include-untracked-noop-on-directories` | 축약된 untracked 디렉터리에 EISDIR → bare `continue` → 플래그가 무음 no-op | 🟠 |
| `include-untracked-silently-drops-files` | 같은 `continue`가 quoted 경로도 삼켜 신규 파일이 경고 없이 사라짐 | 🟠 |

## Reproduction

**symlink 유출** — 직접 재현 확인 (2026-08-05):

```
$ ln -s /etc/hosts leaked-link.txt
$ gz-git diff --include-untracked --format json
diff_content:
  --- /dev/null
  +++ b/leaked-link.txt
  @@ -0,0 +1,10 @@
  +##
  +# Host Database
  +...                    ← /etc/hosts 전문이 그대로 인라인
```

git 자신은 `new file mode 120000` + 링크 **경로만** 출력한다.
`ln -s ~/.aws/credentials creds` 같은 편의용 symlink가 워크트리에 있으면 그 평문이 LLM 프롬프트와 CI 아티팩트에 실린다.

**디렉터리 무음 no-op** — 직접 재현 확인:

```
$ git status --porcelain → ?? newdir/
$ gz-git diff --include-untracked --format json
untracked_files: ["newdir/"]
diff_content: newdir/ 하위 파일 본문 0바이트
exit code 0, stderr 공백, truncated 미설정
```

**메모리** — 191MB untracked 로그 1개, `--max-size 1`(=1KB):
RSS 20MB → **1.21GB**(60배), 결과는 100% 폐기(`truncated=true`, 본문 없음).
`--parallel 3`에서 **4.17GB**. 기본값은 `--parallel 10`.

## Scope

**권장: `os.ReadFile` 루프를 삭제하고 git에 위임한다.**

```
git add -N <untracked paths>   # intent-to-add: 인덱스만 갱신, 커밋하지 않음
git diff                        # 이후 untracked가 정상 diff로 나옴
git reset -- <paths>            # 원복
```

이 한 번의 교체로 symlink(`mode 120000` 표기), 바이너리(`Binary files differ`), 대용량(git 자체 제한), 디렉터리 전개, quoting이 **전부 무료로 해결**된다.

`add -N`을 쓸 수 없는 경우의 최소 수정안:

1. `os.Lstat`으로 선검사 — **regular file만** 처리, symlink/디렉터리/디바이스는 스킵
2. 크기를 `Lstat` 결과로 **먼저** 확인하고 예산 초과 시 읽지 않음
3. 읽기는 `io.LimitReader`로 스트리밍
4. bare `continue`를 제거하고 스킵 사유를 반드시 기록

### 무음 실패 제거 (필수)

현재 bare `continue`는 손실을 어디에도 신호하지 않는다. `omitempty` 신규 키 추가:

```
omitted_files: [{path, reason}]   // reason: "is-directory" | "symlink" | "too-large" | "read-error"
```

## Decision: `git add -N` 권장안을 채택하지 않음

위 "권장" 절의 `add -N` → `diff` → `reset` 3단계는 **거부**하고, 아래 대안을 구현했다.

거부 사유 — `gz-git diff`는 읽기 전용이어야 한다. `add -N`은 인덱스를 변형하며, 이 코드베이스는
`withInterruptCancel`로 SIGINT를 처리하고 기본 `--parallel 10`으로 동작한다. 중단·패닉·타임아웃이
`reset` 이전에 발생하면 **여러 리포에 intent-to-add 엔트리가 남는다**. 사용자가 `diff`를 실행하고
`^C`를 눌렀다는 이유로 인덱스가 오염되는 것은, 고치려는 버그보다 나쁜 부작용이다.

채택한 설계 — **읽기 전용 열거 + 방어적 읽기**:

1. `git ls-files --others --exclude-standard -z` — 디렉터리 전개와 unquoted 경로를 부작용 없이 확보.
   porcelain `??`는 디렉터리를 축약하고 C-quote되므로 열거원으로 쓰지 않는다 (`bulk_diff.go`에서 skip).
2. `os.Lstat` → symlink는 `mode 120000` + 링크 경로만 (git과 동일), 비정규 파일은 `not-regular-file`로 기록.
3. `info.Size()`를 남은 예산과 **먼저** 비교 — 초과 시 읽지 않고 `too-large` 기록 + `Truncated` 설정.
4. `os.Open` → **열린 디스크립터에서 `f.Stat()` 재확인** → `io.LimitReader`.
   Lstat과 Open 사이의 심링크 교체(TOCTOU) 창을 닫는다.
5. bare `continue` 전멸 — 모든 스킵이 `OmittedFiles{Path, Reason}`에 기록된다.

부수 수정: 기존 hunk 빌더가 `strings.Split(content, "\n")`을 써서 개행으로 끝나는 모든 파일에
빈 `+` 줄 하나와 어긋난 라인 수를 냈다. `splitDiffLines`가 이를 처리하고 git의
`\ No newline at end of file` 마커도 붙인다.

`reason` 값은 설계상 3종으로 확정: `not-regular-file` · `too-large` · `read-error`.
초안의 `"is-directory"` / `"symlink"`는 불필요 — 디렉터리는 git이 전개하므로 열거되지 않고,
symlink는 스킵이 아니라 **정상 처리**된다.

## Acceptance Criteria

- [x] untracked symlink가 diff 본문에 **타깃 파일 내용을 인라인하지 않음** (경로/모드만 표기)
      → `TestUntrackedSymlinkIsNotDereferenced`
- [x] untracked 디렉터리 하위 파일 본문이 `--include-untracked`에 정상 포함됨 (중첩 깊이 무관)
      → `TestUntrackedDirectoryIsExpanded` (`docs/adr/` 2단 중첩)
- [x] 공백/비ASCII 경로 untracked 파일이 누락 없이 포함됨 → `TestUntrackedSpacedPathIncluded`
- [x] `--max-size N` 초과 파일에서 프로세스 RSS가 N에 비례해 유지됨
      → `TestUntrackedOversizeFileIsNotRead` (256KB 파일 / `MaxDiffSize` 4096).
      191MB 픽스처는 단위 테스트에 부적합해 비율을 유지한 채 축소했다. 검증 대상은 크기가 아니라
      **읽기 이전에 크기로 거부하는지**이며, `TestReadRegularFileRespectsLimit`이 경계를 직접 확인한다.
- [x] 어떤 이유로든 스킵된 파일이 `omitted_files`에 사유와 함께 보고되고, 사람이 읽는 포맷에도 경고가 표시됨
      → `TestUntrackedUnreadableFileIsReported`; `diff.go`의 `⚠ N untracked file(s) omitted`는
      `--verbose` 무관하게 출력되고 JSON은 `--no-content`에서도 `omitted_files`를 유지한다
- [x] `bulk_diff_test.go:550-568`의 `IncludeUntracked` 테스트 보강
      → `bulk_diff_untracked_test.go` 신규 12건이 실제 읽기 루프를 실행한다 (기존 필드 왕복 테스트는 존치)
- [x] 픽스처 추가: 디렉터리, symlink(리포 밖 시크릿 대상), 바이너리, 대용량, 공백 경로, 개행 없는 파일
      → `untrackedFixture`

## Notes

- git의 untracked 열거는 FIFO·소켓·디바이스 노드를 **애초에 목록에 넣지 않는다** (실측 확인).
  따라서 `not-regular-file` 분기는 일상 경로가 아니라 TOCTOU 경합에 대한 심층 방어다.
  `TestReadRegularFileRejectsDirectory`가 디스크립터 재확인 자체를 검증한다 —
  FIFO로는 테스트할 수 없다. `open()`이 writer를 기다리며 블록되는데, 그것이 바로 이 검사가 막는 위험이다.

## References

- 감사 결과 전문: workflow `wf_6c7e7604-0aa`
- 구현: `pkg/repository/bulk_diff_untracked.go` (신규), `pkg/repository/bulk_diff.go`, `cmd/gz-git/cmd/diff.go`
- 테스트: `pkg/repository/bulk_diff_untracked_test.go` (신규, 12건)
- 우회법은 더 이상 불필요. (수정 전 우회법: `git add -A` 후 `gz-git diff --staged`)
