# ISSUE: golangci-lint 캐시가 삭제된 worktree 경로의 진단을 lint 게이트에 되돌린다

- status: open
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

## 근본 원인

golangci-lint는 분석 결과를 **절대 경로 키**로 캐시한다. 캐시는 `~/Library/Caches/golangci-lint`
같은 사용자 전역 위치에 있으므로 **worktree보다 오래 산다.**

이 저장소의 워크플로가 그 수명 차이를 매 태스크마다 만들어낸다:

1. 태스크 worktree를 만든다 → lint가 그 절대 경로 아래 파일들을 분석·캐시한다
2. `integrate run`이 통합 후 worktree를 삭제한다 (정책상 회수는 통합의 일부다)
3. 다음 `integrate check`의 lint가 캐시를 재사용한다 → **사라진 경로의 진단이 부활한다**

`integrate check`는 그 카운트를 베이스라인으로 읽고 "기존 실패, 악화되지 않음"으로 분류한다.
분류 자체는 게이트를 막지 않지만, **없는 실패를 있다고 보고**하므로 다음 두 가지가 깨진다:

- 사람이 매번 237건을 조사해야 하는지 판단해야 한다 (이번 세션에서 실제로 조사했다)
- 진짜 베이스라인 실패가 유령 카운트에 섞여 구분되지 않는다

## 후보 해법

| 안 | 내용 | 평가 |
| -- | ---- | ---- |
| A | lint 게이트가 **존재하지 않는 경로의 진단을 버린다** | 원인을 안 건드리지만 증상에 정확히 대응. 게이트 코드만 바뀜 |
| B | worktree 회수 시 `golangci-lint cache clean` 실행 | 원인 제거. 다만 전역 캐시를 통째로 날려 다른 저장소의 캐시 히트도 잃는다 |
| C | worktree별로 `GOLANGCI_LINT_CACHE`를 분리하고 회수 시 함께 삭제 | 가장 깨끗하지만 lint 실행 경로 전반에 환경변수 주입 필요 |

A는 게이트의 보고 정확도를, B/C는 캐시의 정합성을 고친다. **A + C 조합**이 유력하다 —
A는 이미 오염된 캐시를 가진 기존 장비에서도 즉시 효과가 있고, C는 재발을 막는다.

## Acceptance Criteria

- [ ] 삭제된 worktree 경로를 가리키는 진단이 lint 베이스라인 카운트에 포함되지 않는다
- [ ] 재현 절차로 검증한다: worktree 생성 → lint 실행 → worktree 삭제 → `integrate check`
      → 베이스라인 카운트가 0
- [ ] 진짜 베이스라인 실패는 여전히 카운트되고 보고된다 (억제 범위가 넓어지지 않았음을 확인)
- [ ] 선택한 해법이 B/C라면 `GOWORK=off`가 필요한 이유와 함께 문서에 남긴다

## 범위 경계

- golangci-lint 자체의 캐시 키 설계를 바꾸려 하지 않는다. 상류 도구의 동작이다.
- `.golangci.yml`의 린터 구성·제외 규칙은 이 태스크에서 건드리지 않는다
  ([21-golangci-exclusion-paths-unanchored](21-golangci-exclusion-paths-unanchored.md) 별건).
- worktree 회수 정책 자체는 유지한다. 회수를 늦추는 방향은 해법이 아니다.

## References

- 증상과 우회법은 사용자 문서에 이미 기록됨:
  `docs/user/guides/troubleshooting.md` → "integrate check reports a lint baseline that is not there"
- 관련: [21-golangci-exclusion-paths-unanchored](21-golangci-exclusion-paths-unanchored.md)
- 회수 정책: worktree/브랜치 제거는 통합 성공과 같은 단계에서 수행한다

## Open Questions

- 유령 카운트가 언제부터 누적됐는지 — 과거 `integrate check` 로그의 베이스라인 카운트가
  태스크 worktree 개수에 비례해 늘었는지 확인하면 영향 범위를 알 수 있다
- `go vet`, `staticcheck` 등 다른 캐시 기반 도구도 같은 문제를 겪는지
