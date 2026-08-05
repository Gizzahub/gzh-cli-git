# ISSUE: LLM 포맷이 맵 키를 정렬 없이 방출해 실행마다 순서가 바뀜

- status: todo
- priority: P3
- category: cross-repo (gzh-cli-core)
- created_at: 2026-08-05T16:00:00+09:00
- affects: v0.7.0 (gz-git `--format llm`)
- spawned_from: `05-diff-output-untracked-visibility.md` (Follow-up #2)

## Background

태스크 05의 골든 테스트 작성 중 발견. `gz-git diff --format llm`의 `SUMMARY:` 블록이
키가 2개 이상일 때 **실행마다 순서가 달라진다.** `-count=20` 재실행에서 3회 실패.

기계 소비(LLM 에이전트)를 전제로 만들어진 포맷에서 비결정성은 실결함이다 — 동일 입력에
대해 동일 출력이어야 diffing/caching/회귀 테스트가 의미를 갖는다.

## Root Cause

`gzh-cli-core/cli/llm_formatter.go:177`:

```go
iter := v.MapRange()        // reflect.Value.MapRange — 순서 보장 없음
for iter.Next() {
    key := iter.Key()
    ...
}
```

`reflect.Value.MapRange`는 Go 맵 순회와 동일하게 **무작위 순서**로 순회한다. 정렬이 없다.

영향 범위: 구조체의 `map[string]...` 필드 전부. `gz-git`에서는 `Summary map[string]int`
(`has-changes`/`clean`/`error` 카운트)가 가장 자주 2개 이상 키를 갖는다.

## Reproduction

```
$ for i in $(seq 1 5); do gz-git diff --format llm <dir> | grep -A3 '^SUMMARY:'; echo --; done
SUMMARY:
  clean: 1
  has-changes: 1
--
SUMMARY:
  has-changes: 1       ← 순서 바뀜
  clean: 1
--
...
```

## Scope (cross-repo)

수정 주체는 **`gzh-cli-core`** 저장소(`github.com/gizzahub/gzh-cli-core`)이므로 본
gitforge 저장소에서 직접 고칠 수 없다. 본 태스크는 추적용.

수정안: `llm_formatter.go`의 맵 분기에서 키를 정렬 후 순회.

```go
keys := make([]string, 0, v.Len())
iter := v.MapRange()
for iter.Next() { keys = append(keys, iter.Key().String()) }
sort.Strings(keys)
for _, k := range keys {
    valueStr := l.formatValue(v.MapIndex(reflect.ValueOf(k)), depth+1)
    ...
}
```

(키 타입이 string이 아닌 경우의 일반화는 구현 시 검토.)

## Acceptance Criteria (gzh-cli-core에서 구현 시)

- [ ] `WriteLLM`이 동일 구조체 입력에 대해 항상 동일 바이트 출력을 생성
- [ ] 맵 키가 정렬 순으로 방출됨
- [ ] `gzh-cli-core/cli/llm_formatter_test.go`에 비결정성 회귀 테스트 추가
      (100회 반복 출력이 모두 일치하는지)
- [ ] gzh-cli-gitforge의 `sortLLMSummaryBlock`(`cmd/gz-git/cmd/diff_output_test.go`)
      정규화 헬퍼 제거 — 원본이 결정적이면 정규화가 불필요

## 현재 우회 (gitforge 측)

`05`의 골든 테스트는 `sortLLMSummaryBlock`으로 `SUMMARY:` 블록을 정렬 정규화한 뒤 비교한다.
이는 임의의 한 순열을 정답으로 굳히는 것을 피하기 위함이지, 원본 결함을 고친 것이 아니다.

## References

- 발견 배경: `05-diff-output-untracked-visibility.md` Decisions #4 / Follow-up #2
- 결함 위치: `gzh-cli-core/cli/llm_formatter.go:177`
- 관련 gzh-cli-core CLAUDE.md: `cli/` 패키지 (Cobra helpers, Output)
