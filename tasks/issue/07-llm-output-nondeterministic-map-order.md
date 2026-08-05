# ISSUE: LLM 포맷이 맵 키를 정렬 없이 방출해 실행마다 순서가 바뀜

- status: blocked (core 수정 완료 / `gzh-cli-core` 릴리스 대기)
- priority: P3
- category: cross-repo (gzh-cli-core)
- created_at: 2026-08-05T16:00:00+09:00
- fixed_at: 2026-08-05T19:40:00+09:00 (gzh-cli-core 작업 트리, **미커밋**)
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
2. **정렬 기준을 "렌더링된 키 문자열"로 잡았다.** 키 타입별 switch(string/int/uint/float)
   대신 이미 방출에 쓰는 문자열을 기준으로 삼는다. 키 타입에 무관하게 결정성이 서고
   정렬 기준과 출력 바이트가 일치한다. 대가는 `map[int]X`가 사전순(`10` < `2`)이라는 것 —
   실사용 맵이 전부 `map[string]...`이고, 비결정성 제거가 목적이므로 수용했다.
3. **값 문자열로 tie-break한다.** `map[any]any`에서 `1`과 `"1"`처럼 서로 다른 키가 같은
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

`golangci-lint`는 실행 불가 — 설치본이 go1.25로 빌드되어 있고 모듈이 go1.26을 타깃한다
(06과 동일한 기존 환경 문제, 이 변경과 무관).

## Acceptance Criteria

- [x] `WriteLLM`이 동일 구조체 입력에 대해 항상 동일 바이트 출력을 생성
- [x] 맵 키가 정렬 순으로 방출됨
- [x] `gzh-cli-core/cli/llm_formatter_test.go`에 비결정성 회귀 테스트 추가
      (`TestLLMFormatter_MapDeterministicOrder` — 8키 맵 100회 렌더 바이트 일치 +
      사전순 확인. 8키로 잡은 이유는 2키에서는 미정렬 구현도 50% 확률로 통과하기 때문)
- [ ] gzh-cli-gitforge의 `sortLLMSummaryBlock`(`cmd/gz-git/cmd/diff_output_test.go`)
      정규화 헬퍼 제거 — **아래 이유로 아직 불가**

## 왜 헬퍼를 지금 제거할 수 없는가 (실험으로 확인)

`go.work`와 CI의 모듈 해석 경로가 다르다.

| 경로 | core 출처 | 근거 |
|------|-----------|------|
| 로컬 개발 | `../gzh-cli-core` 작업 트리 | `gzh-cli-gitforge/go.work`의 `use (. ../gzh-cli-core)` |
| CI | `go.mod` pinned `v0.0.0-20251230045225-725b628c716a` | `.github/workflows/ci.yml`의 `GOWORK: off` (5개 job 전부) |

`go.mod`에 `replace` 지시자는 없다. 헬퍼를 걷어낸 상태로 양쪽을 돌린 결과:

```console
$ go test ./cmd/gz-git/cmd/ -run TestDiffLLM -count=20            # 로컬(수정된 core)
ok

$ GOWORK=off go test ./cmd/gz-git/cmd/ -run TestDiffLLM -count=20 # CI 조건(구 core)
--- FAIL: TestDiffLLMFormatShowsUntracked
    testdata/diff_llm_untracked.golden: line 6 differs
      got:  "  has-changes: 1"
      want: "  clean: 1"
FAIL
```

즉 **core 수정만으로 정규화가 불필요해지는 것은 증명됐고**(로컬 20/20 통과),
제거 시점만 릴리스에 묶여 있다. 지금 제거하면 로컬은 초록, CI는 빨강이 된다.

골든 파일에 기록된 순서가 이미 사전순(`clean` → `has-changes`)이므로
**골든 재기록은 불필요하다.**

## 남은 절차 (순서 고정)

1. `gzh-cli-core` 작업 트리의 변경 2건(`cli/llm_formatter.go`, `cli/llm_formatter_test.go`)을
   커밋 → 푸시. **현재 미커밋 상태다.**
2. `gzh-cli-gitforge`에서 `go get github.com/gizzahub/gzh-cli-core@<new-pseudo-version>`
   으로 `go.mod` 갱신.
3. `GOWORK=off go test ./cmd/gz-git/cmd/ -run TestDiffLLM -count=20` 통과 확인.
4. `sortLLMSummaryBlock`과 그 호출(`diff_output_test.go:311`)을 제거.
   `slices` 임포트가 그 함수 전용이면 함께 정리.
5. 본 태스크 `done` 전환.

## References

- 발견 배경: `05-diff-output-untracked-visibility.md` Decisions #4 / Follow-up #2
- 결함 위치: `gzh-cli-core/cli/llm_formatter.go` `formatMap`
- 관련 gzh-cli-core CLAUDE.md: `cli/` 패키지 (Cobra helpers, Output)
