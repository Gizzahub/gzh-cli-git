# ISSUE: `CHANGELOG.md`가 문서 크기 게이트를 초과해 신규 항목 추가가 차단된다

- status: open
- priority: P2
- category: docs
- created_at: 2026-08-24T23:04:00+09:00
- affects: Unreleased 변경 이력 누락 — 게이트가 막는 동안 착지한 변경은 CHANGELOG에 기록되지 않는다
- spawned_from: `fix/push-stderr` 통합 중 CHANGELOG 항목을 추가하려다 훅에 차단됨

## 요약

`CHANGELOG.md`는 **63019 바이트 / 1320줄**이다. `ce-validate-filesize.sh` 훅의 error 한도는
**46080 바이트 / 500 prose줄**이므로 용량 기준 **1.37배**, 줄 기준 **2배 이상** 초과했다.

훅은 PostToolUse 단계에서 편집을 거부한다:

```console
🟠 CHANGELOG.md
   File size 63902 bytes exceeds error limit 46080 bytes;
   File length 1027 prose lines (1332 total, 305 blank or inside code fences)
   exceeds error limit 500 lines
   💡 Split this file into smaller modules
Error: validation failed with 1 blocking issues
```

## 왜 지금 문제인가

크기 초과 자체보다 **결과**가 문제다. 이 게이트는 CHANGELOG를 *읽기 어렵게* 만드는 데서
멈추지 않고 **쓰기를 막는다.** 2026-08-24에 착지한 두 변경이 그 때문에 기록되지 않았다:

| 커밋      | 변경                                              | CHANGELOG |
| --------- | ------------------------------------------------- | --------- |
| `4b00f90` | bulk push 실패 시 git stderr를 노출 (fix)         | 없음      |
| `e8fad1b` | `info` 범례로 BRANCH/BASE 비교 대상 명시 (added)  | 없음      |

"한도를 넘었으니 추가하지 말라"는 지시는 append-only 파일에 대해서는 성립하지 않는다.
게이트를 끄거나, 파일을 쪼개거나 둘 중 하나여야 하는데 지금은 **조용히 이력이 비는 쪽**으로
기울어 있다.

## 전례 — 줄이는 방식은 유지되지 않는다

[19-tasks-readme-exceeds-doc-size-guideline](19-tasks-readme-exceeds-doc-size-guideline.md)이
`tasks/README.md`에서 **같은 문제**를 겪었고 결론은 다음과 같았다:

> 2026-08-05 시점에 11098바이트로 한 번 초과했고, 그때는 내가 추가한 분량만 줄여
> 10347바이트로 맞췄다. 이틀 만에 두 배가 됐다 — **줄이는 방식으로는 유지되지 않는다.**

해법도 이미 나와 있다: 자라는 부분(끝난 일의 기록)을 별도 파일로 옮기고 인덱스만 남긴다.
CHANGELOG는 그 성질이 더 강하다 — **전량이 "끝난 일의 기록"** 이고 릴리스마다 단조 증가한다.

## Acceptance Criteria

- [ ] `CHANGELOG.md` < 46080 바이트 **그리고** prose 500줄 이하
- [ ] 과거 릴리스 본문이 **삭제되지 않고** 조회 가능하다 (예: `docs/changelog/vN.M.md` 또는
      `CHANGELOG-HISTORY.md`)
- [ ] `CHANGELOG.md`에 `[Unreleased]` + 최신 릴리스가 남고, 과거로 가는 링크가 있다
- [ ] Keep a Changelog / SemVer 헤더 문구가 유지된다 (파일 상단 6줄)
- [ ] 누락된 `4b00f90`·`e8fad1b` 항목이 `[Unreleased]`에 기록된다
- [ ] 분할 후 파일에 실제로 항목을 추가해 보고 훅이 통과하는지 확인한다

## 범위 경계

- 기록 **내용**을 요약·삭제하지 않는다. 위치만 옮긴다.
- 릴리스 자동화(`.make/`, CI)가 `CHANGELOG.md`를 파싱한다면 그 경로도 함께 확인한다.
  확인 없이 파일을 쪼개면 릴리스 노트 생성이 조용히 깨질 수 있다.
- 훅 자체(`ce-validate-filesize.sh`)의 한도는 이 태스크에서 바꾸지 않는다. 개인 정책이
  저장소 바깥에 있으므로 별건이다.

## References

- `CHANGELOG.md` — 63019 바이트 / 1320줄
- 훅: `~/.claude/hooks/scripts/ce-validate-filesize.sh` (error: 46080 바이트 / 500 prose줄)
- 가이드라인: `skill:docs:doc-standards` (guide 기준 500줄 / 45KB error)
- 전례: [19-tasks-readme-exceeds-doc-size-guideline](19-tasks-readme-exceeds-doc-size-guideline.md)

## Open Questions

- 분할 단위를 **릴리스별**(`docs/changelog/v0.7.0.md`)로 할지 **연도/분기별**로 할지.
  릴리스별은 파일 수가 늘지만 각 파일이 다시 자라지 않는다.
- **릴리스별 분할만으로는 부족할 가능성이 크다.** `[Unreleased]` 섹션 하나가 이미 445줄
  (8~452행)로 500줄 한도에 근접해 있다. 과거 릴리스를 전부 옮겨도 `CHANGELOG.md`는
  한도 언저리에서 시작한다. `[Unreleased]`를 릴리스로 확정해 잘라내는 절차가
  분할과 함께 필요한지 판단해야 한다.
- 위와 연결해서: `[Unreleased]`가 445줄까지 자란 것 자체가 릴리스 주기 문제의 신호인지.
