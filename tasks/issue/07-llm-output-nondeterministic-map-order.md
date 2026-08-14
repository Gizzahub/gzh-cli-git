# ISSUE: LLM 포맷이 맵 키를 정렬 없이 방출해 실행마다 순서가 바뀜

- status: done
- priority: P3
- category: cross-repo (gzh-cli-core)
- created_at: 2026-08-05T16:00:00+09:00
- fixed_at: 2026-08-05T19:40:00+09:00 (gzh-cli-core `84a0f3d`, published)
- closed_at: 2026-08-14 (gitforge `51aadf0`: bump the standalone-module dependency)
- affects: v0.7.0 (gz-git `--format llm`)
- spawned_from: `05-diff-output-untracked-visibility.md` (Follow-up #2)

## Background

태스크 05의 골든 테스트 작성 중 발견. `gz-git diff --format llm`의 `SUMMARY:` 블록이
키가 2개 이상일 때 **실행마다 순서가 달라진다.** `-count=20` 재실행에서 3회 실패.

기계 소비(LLM 에이전트)를 전제로 만들어진 포맷에서 비결정성은 실결함이다 — 동일 입력에
대해 동일 출력이어야 diffing/caching/회귀 테스트가 의미를 갖는다.

## Root Cause

`gzh-cli-core/cli/llm_formatter.go:177` (수정 전):

```go
iter := v.MapRange()        // reflect.Value.MapRange — 순서 보장 없음
for iter.Next() {
    key := iter.Key()
    ...
}
```

`reflect.Value.MapRange`는 Go 맵 순회와 동일하게 **무작위 순서**로 순회한다. 정렬이 없다.
core 전체에서 맵을 순회하는 지점은 이 한 곳뿐이라(`grep MapRange` 1건) 수정 지점도 하나다.

영향 범위: 구조체의 `map[string]...` 필드 전부. `gz-git`에서는 `Summary map[string]int`
(`has-changes`/`clean`/`error` 카운트)가 가장 자주 2개 이상 키를 갖는다.

## 구현 (gzh-cli-core, 2026-08-05)

`cli/llm_formatter.go`의 `formatMap`을 정렬 방출로 교체하고 `sort` 임포트를 추가했다.

```go
type mapEntry struct{ key, value string }

entries := make([]mapEntry, 0, v.Len())

for _, key := range v.MapKeys() {
    valueStr := l.formatValue(v.MapIndex(key), depth+1)
    if valueStr == "" {
        continue
    }
    entries = append(entries, mapEntry{key: l.formatValue(key, depth), value: valueStr})
}

sort.Slice(entries, func(i, j int) bool {
    if entries[i].key != entries[j].key {
        return entries[i].key < entries[j].key
    }
    return entries[i].value < entries[j].value
})
```

### 설계 판단 3건

1. **원 태스크의 스케치를 쓰지 않았다.** 초안은
   `v.MapIndex(reflect.ValueOf(k))`로 되돌아가는 형태였는데, 키가 named type
   (`type State string`)이면 `reflect.ValueOf(k)`가 plain `string` Value를 만들어
   `MapIndex`가 타입 불일치로 **panic**한다. `v.MapKeys()`가 돌려주는 `reflect.Value`를
   그대로 들고 있으면 이 경로 자체가 사라진다. 회귀 테스트로 고정
   (`TestLLMFormatter_MapNamedKeyType`).
1. **정렬 기준을 "렌더링된 키 문자열"로 잡았다.** 키 타입별 switch(string/int/uint/float)
   대신 이미 방출에 쓰는 문자열을 기준으로 삼는다. 키 타입에 무관하게 결정성이 서고
   정렬 기준과 출력 바이트가 일치한다. 대가는 `map[int]X`가 사전순(`10` < `2`)이라는 것 —
   실사용 맵이 전부 `map[string]...`이고, 비결정성 제거가 목적이므로 수용했다.
1. **값 문자열로 tie-break한다.** `map[any]any`에서 `1`과 `"1"`처럼 서로 다른 키가 같은
   문자열로 렌더링되면 정렬만으로는 순서가 다시 불안정해진다. 값은 어차피 방출 전에
   계산하므로(빈 값 skip 판정 때문에) 추가 비용 없이 완전 결정성을 얻는다.

### 검증

```console
$ go test ./cli/ -run TestLLMFormatter -count=20     # ok
$ make test                                          # 6개 패키지 전부 ok (cli 커버리지 73.6%)
```

**역검증** — `sort.Slice` 호출만 제거하고 재실행하면 `render 1`에서 즉시 실패한다.
테스트가 실제로 이 결함을 잡는다는 것을 확인한 뒤 복원했다.

```
--- FAIL: TestLLMFormatter_MapDeterministicOrder
    render 1 differs from the first render:
    first:  SUMMARY:\n  clean: 2\n  error: 3\n  skipped: 4 ...
    got:    SUMMARY:\n  skipped: 4\n  ahead: 5\n  behind: 6 ...
```

초기 검증 시점에는 로컬 `golangci-lint` 실행 환경이 준비되지 않았지만, 이는 이 변경의
정합성이나 회귀 테스트와 무관한 환경 제약이었다. 현재 lint 게이트의 상태와 잔여 정리는
[issue 21](21-golangci-exclusion-paths-unanchored.md)에 기록한다.

## Acceptance Criteria

- [x] `WriteLLM`이 동일 구조체 입력에 대해 항상 동일 바이트 출력을 생성
- [x] 맵 키가 정렬 순으로 방출됨
- [x] `gzh-cli-core/cli/llm_formatter_test.go`에 비결정성 회귀 테스트 추가
  (`TestLLMFormatter_MapDeterministicOrder` — 8키 맵 100회 렌더 바이트 일치 +
  사전순 확인. 8키로 잡은 이유는 2키에서는 미정렬 구현도 50% 확률로 통과하기 때문)
- [x] gzh-cli-gitforge의 `sortLLMSummaryBlock`(`cmd/gz-git/cmd/diff_output_test.go`)
  정규화 헬퍼 제거 — **2026-08-07** 완료 (raw golden assert)

## 완료 노트 (2026-08-14)

`sortLLMSummaryBlock` 제거 완료. `TestDiffLLMFormatShowsUntracked`는 raw output으로
`assertGolden` 호출. 골든은 이미 사전순(`clean` → `has-changes`).

| 경로                          | core 출처                                                    | 결과                                                           |
| ----------------------------- | ------------------------------------------------------------ | -------------------------------------------------------------- |
| 로컬 / devbox `go.work`       | `./gzh-cli-core` (sorted `formatMap`)                        | `go test ./cmd/gz-git/cmd/ -run Diff\|LLM\|Golden -count=3` ok |
| CI / 단독 모듈 (`GOWORK=off`) | `go.mod` → `gzh-cli-core@v0.0.0-20260805234833-84a0f3d05f5d` | published core를 소비하도록 bump 완료 (`51aadf0`)              |

소비자 단독 CI에서 정렬된 core를 사용하도록 `go.mod`/`go.sum`을 갱신했고, 이제
`GOWORK=off` 경로도 로컬 `go.work`와 같은 formatter 구현을 사용한다. 따라서 이 태스크의
후속( core publish 및 consumer bump)은 종료되었다.

## References

- 발견 배경: `05-diff-output-untracked-visibility.md` Decisions #4 / Follow-up #2
- 결함 위치: `gzh-cli-core/cli/llm_formatter.go` `formatMap`
- 관련 gzh-cli-core CLAUDE.md: `cli/` 패키지 (Cobra helpers, Output)
