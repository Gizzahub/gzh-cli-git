# ISSUE: golangci-lint 캐시가 삭제된 worktree 경로의 진단을 lint 게이트에 되돌린다

- status: done (2026-08-24 — `e7f631f`/`052325e`에서 이미 해결됨. 관측 당시 설치 바이너리가 낡아 재현된 것)
- priority: P2
- category: build / quality gate
- created_at: 2026-08-24T23:04:00+09:00
- affects: `gz-git integrate check` — 실재하지 않는 lint 베이스라인 실패를 보고한다
- spawned_from: `dev/claude/mst/fix/push-stderr` 통합 중 유령 베이스라인 237건을 확인

## 요약

`integrate check`가 lint 베이스라인 실패를 보고했다:

```console
WARN make lint — baseline failure, non-worsening: count 237 → 237,
     no diagnostics on changed paths
```

그런데 237건의 진단이 가리키는 경로는 **그 시점에 이미 존재하지 않는 디렉토리**였다:

```console
../../../worktrees/gzh-cli/gzh-cli-gitforge/claude__mst__fix__push-stderr/pkg/...
```

직전 `integrate run`이 통합 성공 후 정책대로 회수(reclaim)한 worktree다. 캐시를 비우자
같은 커밋에서 결과가 뒤집혔다:

```console
$ golangci-lint cache clean
$ GOWORK=off make lint
0 issues.
```

즉 master의 lint는 깨끗했고, 237은 **캐시가 만들어낸 유령**이었다.

## 원인 (1) — golangci-lint 캐시가 worktree보다 오래 산다

golangci-lint는 분석 결과를 **절대 경로 키**로 캐시한다. 캐시는 사용자 전역 위치에 있으므로
worktree가 사라져도 남는다. 이 저장소의 워크플로가 그 수명 차이를 매 태스크마다 만든다:

1. 태스크 worktree를 만든다 → lint가 그 절대 경로 아래 파일들을 분석·캐시한다
2. `integrate run`이 통합 후 worktree를 삭제한다 (회수는 통합의 일부다)
3. 다음 `integrate check`의 lint가 캐시를 재사용한다 → **사라진 경로의 진단이 부활한다**

## 원인 (2) — 게이트를 고쳐도 `make install` 전까지는 낡은 게이트가 판정한다

**이쪽이 이 이슈의 실제 교훈이다.** 위 (1)은 이 이슈를 파일링하기 **11시간 전에 이미
수정되어 master에 있었다.**

| 커밋 | 시각 | 내용 |
| ---- | ---- | ---- |
| `e7f631f` | 2026-08-24 11:37 | `fix(pkg/integrate): isolate lint gate caches` |
| `052325e` | 2026-08-24 11:43 | `fix(pkg/integrate): harden lint path validation` |

그럼에도 20:35~22:52 사이의 `integrate check`는 유령 237건을 계속 보고했다. `integrate
check`를 실행하는 주체가 **소스 트리가 아니라 설치된 `~/go/bin/gz-git`** 이기 때문이다.
그날 `make install`을 실행한 시각은 23:02이고, 그 이후 실행부터 `PASS make lint — ok`가
나왔다. 설치 바이너리에 새 코드가 들어갔는지는 다음으로 확인된다:

```console
$ strings ~/go/bin/gz-git | grep -c "diagnostics reference paths outside the repository"
1
```

게이트 코드를 고치는 사람은 항상 소스 트리에서 일하는데 판정은 설치 바이너리가 한다.
자기 자신을 검사하는 도구는 이 이중성을 구조적으로 갖는다.

## Resolution (2026-08-24)

`e7f631f`가 두 가지를 동시에 넣었고, 그것이 이 문서가 후보로 적었던 두 방향 그대로다.

**실행마다 격리된 캐시** — `pkg/integrate/check_make.go`:

```go
lintCache, err = os.MkdirTemp("", "gz-git-integrate-golangci-lint-")
defer func() { _ = os.RemoveAll(lintCache) }()
cmd.Env = append(cmd.Env, "GOLANGCI_LINT_CACHE="+lintCache)
```

lint 타깃에만 적용되며 실행이 끝나면 지운다. 전역 캐시를 건드리지 않으므로 다른 저장소의
캐시 히트를 잃지 않는다.

**저장소 밖 진단은 베이스라인이 아니라 실패** — `pkg/integrate/check_baseline.go`의
`foreignDiagnosticLocations`가 `../`로 시작하는 진단 위치를 찾아내고, `judgeMake`가 그것을
`checkFail`로 만든다:

```text
branch diagnostics reference paths outside the repository: ...
```

"비악화 베이스라인"으로 조용히 넘어가는 경로가 사라졌다.

## 남은 것

- 게이트 밖에서 사람이 직접 실행하는 `make lint`는 여전히 전역 캐시를 쓴다. 유령 진단을
  보면 `golangci-lint cache clean`이 필요하다 — 증상과 우회법은
  `docs/user/guides/troubleshooting.md`에 남겨 두었다.
- `make install` 지연 문제는 같은 문서의 "A gate fix does not take effect until
  `make install`" 절에 기록했다.

## References

- `pkg/integrate/check_make.go` — per-run `GOLANGCI_LINT_CACHE`
- `pkg/integrate/check_baseline.go` — `foreignDiagnosticLocations` / `foreignDiagnosticError`
- `docs/user/guides/troubleshooting.md`
- 관련: [21-golangci-exclusion-paths-unanchored](21-golangci-exclusion-paths-unanchored.md)
  (제외 패턴 앵커링 — 캐시와는 별개 사안)
