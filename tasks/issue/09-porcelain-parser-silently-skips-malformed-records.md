# ISSUE: `parsePorcelainZ`가 깨진 레코드를 조용히 버린다 (구 파서는 에러였음)

- status: done
- priority: P2
- category: repository
- created_at: 2026-08-05T21:40:00+09:00
- affects: v0.7.0+ (태스크 06에서 유입)
- spawned_from: `/quality:review:session` (06 완료 직후 세션 리뷰)

## Background

태스크 06이 `internal/parser.ParseStatus`를 지우고 `pkg/repository/porcelain.go`로
통합할 때, **파싱 불가 입력에 대한 계약이 조용히 바뀌었다.** 구 파서는 형식에 맞지
않는 라인에 에러를 돌려줬는데, 신규 파서는 건너뛴다. CHANGELOG에도 기재되지 않았다.

## Findings

`pkg/repository/porcelain.go:51`

```go
for i := 0; i < len(records); i++ {
    // "XY PATH": two status characters, a space, then at least one byte of
    // path. The trailing NUL leaves an empty final record that lands here too.
    if len(records[i]) < 4 {
        continue
    }
```

`continue`가 처리하는 입력은 두 종류인데 코드가 구분하지 않는다.

| 입력 | 실제 의미 | 올바른 처리 |
|---|---|---|
| 마지막 NUL 뒤의 빈 문자열 `""` | 정상 — `-z`는 레코드마다 NUL을 붙이므로 항상 생긴다 | 건너뛰기 (현행 유지) |
| `"M"`, `"XY"`, `"?? "` 등 4바이트 미만 비어있지 않은 값 | **비정상** — git이 이런 걸 낼 이유가 없다. 냈다면 우리가 모르는 포맷이거나 stdout이 잘린 것 | 에러 |

즉 현행 코드는 "정상적으로 예상되는 빈 레코드"를 처리하려다 **"우리가 포맷을 잘못
알고 있다"는 유일한 신호까지 같이 삼킨다.** 전역 규칙(`error-visibility`: 오류
숨김/무시/삼킴 금지, fail-fast)에 정면으로 어긋난다.

### 왜 지금 중요한가

같은 함수의 rename 페어링 결함(` R` 미처리)이 P0였던 이유가 정확히 이것이다 —
소스 경로가 레코드 스트림에 남아 상태 라인으로 재해석됐다. 그때는 경로가 4바이트
이상이라 `applyStatusCode`까지 도달해 `unknown index status code: h`로 **터졌기
때문에** 발견됐다. 만약 소스 경로가 `a.c`(3바이트)였다면 이 `continue`가 조용히
먹어치웠고, 결함은 "파일 하나가 이유 없이 목록에서 사라진다"로만 나타났을 것이다.

## Scope

1. 빈 레코드와 형식 위반을 분리한다.
   ```go
   if records[i] == "" {
       continue // trailing NUL
   }
   if len(records[i]) < 4 {
       return nil, fmt.Errorf("malformed porcelain record %q", records[i])
   }
   ```
2. `parsePorcelainZ`가 `([]porcelainRecord, error)`를 반환하도록 시그니처 변경.
   호출자 3곳(`collectChangeSet`, `GetStatus`, `checkRepositoryState`)은 이미
   `statusFromRecords`의 에러를 감싸고 있으므로 분기 추가 부담이 작다.
3. rename 소스 레코드 lookahead(`i+1 < len(records)`)가 **거짓일 때도** 에러여야 한다.
   현행은 소스 없는 rename을 `OldPath: ""`로 조용히 통과시킨다.
4. CHANGELOG에 계약 변경으로 기재 (구 `ParseStatus`는 에러였다는 사실 포함).

## Acceptance Criteria

- [x] 비어있지 않은 4바이트 미만 레코드에 대해 non-nil error
- [x] 후행 NUL로 생기는 빈 레코드는 여전히 에러 없이 통과
- [x] rename 코드인데 다음 레코드가 없으면 non-nil error
- [x] 위 3건 각각 테이블 테스트 케이스 보유 (06에서 삭제된 "short record" 케이스 복원)
- [x] CHANGELOG 기재

테이블 케이스 4건 추가: `trailing NUL does not become a phantom record`,
`record too short to hold a status code`(`"M"`),
`record with status code but no path`(`"?? "`), `rename with no source record`.

## Decisions

### Scope 3의 lookahead 가드는 길이 검사만으로 부족했다

태스크가 제안한 `i+1 < len(records)`는 **항상 참**이다. `-z`는 모든 레코드를 NUL로
끝내므로 `strings.Split`이 언제나 빈 원소 하나를 뒤에 남긴다. 소스가 잘린 rename
(`"R  new.go\x00"` → `["R  new.go", ""]`)은 길이 검사를 통과한 뒤 그 빈 원소를
`OldPath: ""`로 채택한다 — 태스크가 막으려던 바로 그 조용한 통과다.

가드는 `i+1 >= len(records) || records[i+1] == ""`로 확정했다. git은 빈 경로를 내지
않으므로 빈 소스 레코드를 거부해도 정상 입력을 잃지 않는다.

같은 빈 문자열이 루프 상단에서는 정상(후행 NUL), lookahead에서는 비정상(잘린 소스)
이라는 점이 이 결함의 구조다 — 한 값이 위치에 따라 두 의미를 갖는데 한쪽만
처리했다. 회귀 방지를 위해 가드를 되돌려 `rename with no source record` 케이스가
실제로 실패하는 것을 확인한 뒤 복원했다.

### `parseStatusZ` 합성 헬퍼 추가

`parsePorcelainZ`가 2값을 반환하게 되자 `GetStatus`와 `checkRepositoryState`가
동일한 2단계 + 동일 문구 래핑을 복제하게 됐다. `parseStatusZ(stdout) (*Status, error)`
로 묶어 호출부를 1단계로 되돌렸다. `collectChangeSet`은 레코드의 XY 쌍이 필요해
(Status는 이를 합집합으로 뭉갠다) 여전히 `parsePorcelainZ`를 직접 쓴다.

## References

- 유입 태스크: `06-porcelain-parsers-outside-diff-commit.md`
- 같은 함수의 P0 결함(수정 완료): rename 페어링 — 이 `continue`가 왜 위험한지의 실례
- 전역 규칙: `error-visibility` (fail-fast), `dev-patterns` (quick fix로 근본원인 우회 금지)
