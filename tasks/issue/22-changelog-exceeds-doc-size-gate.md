# ISSUE: `CHANGELOG.md`가 문서 크기 게이트를 초과해 신규 항목 추가가 차단된다

- status: open
- priority: P1
- category: docs
- created_at: 2026-08-24T23:04:00+09:00
- affects: 릴리스 기록 부재 — 2026-08-07 이후 feat/fix 68커밋이 CHANGELOG에 없다
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

## 이 파일은 이미 17일째 죽어 있다

크기 초과 자체보다 **결과**가 문제다. 이 게이트는 CHANGELOG를 *읽기 어렵게* 만드는 데서
멈추지 않고 **쓰기를 막는다.** 한도를 넘은 시점과 갱신이 멈춘 시점이 정확히 일치한다:

| 커밋 | 날짜 | 크기 | 판정 |
| --------- | ---------- | ------------- | -------------------- |
| `ef24b6f` | 2026-08-07 | 45359 바이트 | 한도 이내 |
| `dd0b0ba` | 2026-08-07 | **46149 바이트** | 한도(46080) 최초 초과 |
| `c3231f7` | 2026-08-07 | 63019 바이트 | **마지막 갱신 — 이후 정지** |

44개 커밋 동안 기능 커밋마다 성실히 갱신되던 파일이, 한도를 넘은 그 주에 멈췄다.
`c3231f7..master` 구간의 **feat/fix 커밋은 68개**이고 그중 어느 것도 기록되지 않았다.
여기에는 사용자 대상 기능이 포함된다:

| 커밋 | 변경 | CHANGELOG |
| --------- | ---------------------------------------------- | --------- |
| `c47963a` | `--sync-base` — stale base ref 복구 (901줄 추가) | 없음 |
| `e8fad1b` | `info` 범례로 BRANCH/BASE 비교 대상 명시 | 없음 |
| `4b00f90` | bulk push 실패 시 git stderr를 노출 | 없음 |

한 사람의 부주의가 아니다. 서로 다른 세션이 같은 벽에 부딪혔다.

"한도를 넘었으니 추가하지 말라"는 지시는 append-only 파일에 대해 성립하지 않는다.
**차단형 게이트를 단조 증가 파일에 걸면 파일이 죽는다.** 경고였다면 68건이 쌓이지 않았다.

## 전례 — 줄이는 방식은 유지되지 않는다

[19-tasks-readme-exceeds-doc-size-guideline](19-tasks-readme-exceeds-doc-size-guideline.md)이
`tasks/README.md`에서 **같은 문제**를 겪었고 결론은 다음과 같았다:

> 2026-08-05 시점에 11098바이트로 한 번 초과했고, 그때는 내가 추가한 분량만 줄여
> 10347바이트로 맞췄다. 이틀 만에 두 배가 됐다 — **줄이는 방식으로는 유지되지 않는다.**

해법도 이미 나와 있다: 자라는 부분(끝난 일의 기록)을 별도 파일로 옮기고 인덱스만 남긴다.
CHANGELOG는 그 성질이 더 강하다 — **전량이 "끝난 일의 기록"** 이고 릴리스마다 단조 증가한다.

## 분할 실측 — 릴리스 컷 없이도 게이트를 통과한다

과거 릴리스만 들어내면 남는 부분이 한도 안에 들어온다. 실측:

| 구간 | 줄 | 바이트 |
| ---------------------------------- | ---- | ------- |
| 1~452행 (헤더 + `[Unreleased]`) | 452 | 31803 |
| 453행~끝 (`[0.7.0]` 이하 8개 릴리스) | 868 | 31216 |

즉 **릴리스를 새로 끊지 않아도** 과거 릴리스 이동만으로 31803바이트 / 452줄(prose는 그보다
적다)이 되어 게이트를 통과한다. 여유는 약 14KB다.

옮길 쪽도 한 파일에 몰면 868줄이라 같은 한도에 걸린다. 마이너 버전 단위로 나눈다:

```text
docs/changelog/0.7.md   (453~473)
docs/changelog/0.6.md   (474~531)
docs/changelog/0.4.md   (532~599)
docs/changelog/0.3.md   (600~946)   ← 가장 큼, 347줄
docs/changelog/0.2.md   (947~984)
docs/changelog/0.1.md   (985~)
```

## 68건 백로그는 손으로 다시 쓰지 않는다

`.goreleaser.yaml`이 이미 커밋 기반으로 GitHub Release 노트를 생성한다
(`changelog.use: github`, Features/Bug fixes 그룹, `^docs:`·`^test:`·`^chore:`·`^ci:` 제외).
**기계적 목록은 이미 존재한다.** 그렇다면 `CHANGELOG.md`가 68건을 다시 손으로 나열할 이유가
없다. 이 파일의 고유한 가치는 "무엇이 바뀌었나"가 아니라 **"왜 그렇게 바꿨나"** 의 서술이다.

따라서 백로그는 주제별로 묶어 주목할 변경만 서술하고, 전량 목록은 릴리스 노트에 위임한다.

## Acceptance Criteria

- [ ] `CHANGELOG.md` < 46080 바이트 **그리고** prose 500줄 이하
- [ ] 과거 릴리스 본문이 **삭제되지 않고** `docs/changelog/`에서 조회 가능하다
- [ ] 분할된 각 파일도 개별적으로 게이트를 통과한다
- [ ] `CHANGELOG.md`에 `[Unreleased]`와 과거 릴리스로 가는 인덱스가 남는다
- [ ] Keep a Changelog / SemVer 헤더 문구(1~6행)와 하단 링크 참조가 유지된다
- [ ] 68건 백로그 중 사용자 대상 변경이 주제별로 `[Unreleased]`에 서술된다
- [ ] 분할 후 파일에 실제로 항목을 추가해 보고 훅이 통과하는지 확인한다
- [ ] 항목 밀도 규칙(항목당 3~5줄, 상세는 `docs/` 링크)이 문서에 명시된다

## 범위 경계

- 기록 **내용**을 요약·삭제하지 않는다. 과거 릴리스는 위치만 옮긴다.
- 릴리스를 새로 끊는 일(태그·배포)은 이 태스크에 포함하지 않는다. 실측대로 분할만으로
  게이트를 통과하므로 릴리스 컷은 필요조건이 아니다.
- `make changelog`(`git-chglog -o CHANGELOG.md`)는 이 파일을 **덮어쓴다.** 분할 구조와
  정면 충돌하므로 [24-make-changelog-overwrites-handwritten-log](24-make-changelog-overwrites-handwritten-log.md)에서
  별도로 처리한다. 그 결론이 나기 전에는 `make changelog`를 실행하지 않는다.
- 훅 자체(`ce-validate-filesize.sh`)의 한도는 이 태스크에서 바꾸지 않는다. 개인 정책이
  저장소 바깥에 있으므로 별건이다.

## References

- `CHANGELOG.md` — 63019 바이트 / 1320줄, 마지막 갱신 `c3231f7` (2026-08-07)
- 훅: `ce-validate-filesize.sh` (error: 46080 바이트 / 500 prose줄)
- 가이드라인: `skill:docs:doc-standards` (guide 기준 500줄 / 45KB error)
- `.goreleaser.yaml` — `changelog.use: github`
- 전례: [19-tasks-readme-exceeds-doc-size-guideline](19-tasks-readme-exceeds-doc-size-guideline.md)
- 후속: [24-make-changelog-overwrites-handwritten-log](24-make-changelog-overwrites-handwritten-log.md)

## Open Questions

- `[Unreleased]`가 445줄까지 자란 것 자체가 릴리스 주기 문제의 신호인지. 분할 후 여유가
  14KB뿐이므로, 항목 밀도 규칙이 없으면 같은 문제가 다시 온다.
