# ISSUE: `goheader` 규칙이 저장소의 모든 파일을 거부한다

- status: todo
- priority: P3
- category: build
- created_at: 2026-08-06T14:40:00+09:00
- affects: v0.7.0+ (설정 도입 시점부터)
- spawned_from: 태스크 15 착수 중 `make lint` 지적 건수가 실행마다 달라지는 것을 추적

## Background

`make lint`의 지적 건수가 실행마다 18·19·22로 흔들렸다. 코드가 바뀌어서가 아니라
`goheader`가 **모든 파일을 위반으로 보고 있고**, golangci-lint가 그중 5건만 표본으로
보여주기 때문이다(`max-same-issues: 5`).

## Findings

### 규칙이 기대하는 회사명이 저장소에 한 번도 등장하지 않는다

`.golangci.yml:110`

```yaml
goheader:
  values:
    const:
      COMPANY: Archmagece
      LICENSE: MIT
  template: |-
    Copyright (c) {{ YEAR }} {{ COMPANY }}
    SPDX-License-Identifier: {{ LICENSE }}
```

실측:

| | 파일 수 |
|---|---|
| `Copyright (c) 20xx Gizzahub` | 155 |
| `Copyright (c) 20xx Archmagece` | **0** |
| 헤더가 아예 없음 | 43 |
| 비테스트 `.go` 합계 | 198 |

**198개 중 통과하는 파일이 하나도 없다.** 테스트 파일은 `.golangci.yml:340`의 exclusion에
걸려 검사 대상이 아니다.

### 아무도 보지 못한다

`make quality`는 `lint-check`를 의도적으로 제외한다(`.make/quality.mk:314`). 프로젝트
CLAUDE.md가 커밋 전 필수로 지정한 것은 `make quality`이므로, 이 규칙의 위반은 별도로
`make lint`를 돌린 사람에게만 보인다. 그마저도 5건씩만 보여 **전수 위반이 아니라
"몇 개 파일의 문제"처럼 읽힌다.**

지적 건수가 흔들리는 것이 부작용이다 — 어떤 커밋이 lint를 나아지게 했는지 나빠지게
했는지 건수로 판단할 수 없다.

## Scope

1. 아래 D1을 확정한다 (`Gizzahub` vs `Archmagece`).
2. 결정에 따라 **한쪽으로 통일**한다.
   - `Gizzahub`이면: `.golangci.yml`의 `COMPANY` 수정 (1줄).
   - `Archmagece`면: 155개 파일 헤더 일괄 치환.
3. 헤더 없는 43개 파일에 헤더를 추가하거나, 그 경로를 exclusion에 넣는다.
   `cmd/gz-git/` 하위가 다수다.
4. `make quality`가 `lint-check`를 부르지 않는 것이 의도인지 재확인한다. 의도라면
   그 이유를 `.make/quality.mk`에 주석으로 남긴다 — 지금은 제외 자체가 근거 없이
   보인다. (별건이지만 이 결함이 숨어 있던 이유이므로 함께 다룬다.)

## Acceptance Criteria

- [ ] `make lint`의 `goheader` 지적 0건
- [ ] `make lint` 총 지적 건수가 연속 3회 실행에서 동일 (표본 추출로 인한 흔들림 제거)
- [ ] D1의 근거가 이 문서에 기록됨

## Decisions

### D1. 필요한 결정: 저작권자 표기 — `Gizzahub` / `Archmagece`

이것은 코드 스타일이 아니라 **귀속(attribution) 표기**이므로 임의로 정하지 않는다.

관찰된 근거는 `Gizzahub` 쪽으로 기운다:

- 모듈 경로가 `github.com/gizzahub/gzh-cli-gitforge`
- 저장소의 155개 파일이 전부 `Gizzahub`
- `.golangci.yml`의 `Archmagece`는 다른 곳에서 복사해 온 설정으로 보인다 —
  이 저장소의 어떤 파일과도 일치하지 않는다

그러나 실제 저작권 귀속은 저장소 소유자만 답할 수 있다. 착수자는 소유자에게 확인한 뒤
근거를 이 절에 기록한다.

## References

- 설정: `.golangci.yml:110-119`(규칙), `:340-341`(테스트 파일 제외),
  `:398-399`(`max-issues-per-linter: 10`, `max-same-issues: 5`)
- 관련: `.make/quality.mk:314` — `make quality`가 `lint-check`를 제외하는 지점
- 태스크 13 D4·태스크 15 D3이 "잔여 N건은 미수정 파일"이라 적은 그 N이
  이 표본 추출 때문에 흔들린 값이다
