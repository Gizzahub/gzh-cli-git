# ISSUE: `CHANGELOG.md`가 문서 크기 게이트를 초과해 신규 항목 추가가 차단된다

- status: done (2026-08-24 — 과거 릴리스를 `docs/changelog/`로 분할, 71커밋 백로그 서술, 밀도 규칙과 예산 명시. 예산은 2026-08-25에 700→1000으로 상향)
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

| 커밋      | 날짜       | 크기             | 판정                        |
| --------- | ---------- | ---------------- | --------------------------- |
| `ef24b6f` | 2026-08-07 | 45359 바이트     | 한도 이내                   |
| `dd0b0ba` | 2026-08-07 | **46149 바이트** | 한도(46080) 최초 초과       |
| `c3231f7` | 2026-08-07 | 63019 바이트     | **마지막 갱신 — 이후 정지** |

44개 커밋 동안 기능 커밋마다 성실히 갱신되던 파일이, 한도를 넘은 그 주에 멈췄다.
`c3231f7..master` 구간의 **feat/fix 커밋은 68개**이고 그중 어느 것도 기록되지 않았다.
여기에는 사용자 대상 기능이 포함된다:

| 커밋      | 변경                                             | CHANGELOG |
| --------- | ------------------------------------------------ | --------- |
| `c47963a` | `--sync-base` — stale base ref 복구 (901줄 추가) | 없음      |
| `e8fad1b` | `info` 범례로 BRANCH/BASE 비교 대상 명시         | 없음      |
| `4b00f90` | bulk push 실패 시 git stderr를 노출              | 없음      |

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

| 구간                                 | 줄  | 바이트 |
| ------------------------------------ | --- | ------ |
| 1~452행 (헤더 + `[Unreleased]`)      | 452 | 31803  |
| 453행~끝 (`[0.7.0]` 이하 8개 릴리스) | 868 | 31216  |

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

- [x] `CHANGELOG.md` < 46080 바이트 **그리고** prose 500줄 이하
- [x] 과거 릴리스 본문이 **삭제되지 않고** `docs/changelog/`에서 조회 가능하다
- [x] 분할된 각 파일도 개별적으로 게이트를 통과한다
- [x] `CHANGELOG.md`에 `[Unreleased]`와 과거 릴리스로 가는 인덱스가 남는다
- [x] Keep a Changelog / SemVer 헤더 문구(1~6행)와 하단 링크 참조가 유지된다
- [x] 68건 백로그 중 사용자 대상 변경이 주제별로 `[Unreleased]`에 서술된다
- [x] 분할 후 파일에 실제로 항목을 추가해 보고 훅이 통과하는지 확인한다
- [x] 항목 밀도 규칙(항목당 3~5줄, 상세는 `docs/` 링크)이 문서에 명시된다

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

## Resolution (2026-08-24)

### 한 일

과거 릴리스 6개 라인을 `docs/changelog/`로 옮기고, `CHANGELOG.md`에는 `[Unreleased]`와
인덱스만 남겼다. 본문은 한 줄도 요약하거나 삭제하지 않았다 — 원본의 453~1298행이 그대로
아래 파일들로 이동했을 뿐이다.

| 파일                    | 포함 릴리스                    | 크기           | 게이트         |
| ----------------------- | ------------------------------ | -------------- | -------------- |
| `docs/changelog/0.7.md` | 0.7.0                          | 26줄 / 1236B   | PASS           |
| `docs/changelog/0.6.md` | 0.6.1, 0.6.0                   | 63줄 / 2383B   | PASS           |
| `docs/changelog/0.4.md` | 0.4.0                          | 73줄 / 2905B   | PASS           |
| `docs/changelog/0.3.md` | 0.3.1, 0.3.0                   | 354줄 / 13359B | PASS (warning) |
| `docs/changelog/0.2.md` | 0.2.0                          | 45줄 / 2127B   | PASS           |
| `docs/changelog/0.1.md` | 0.1.0-alpha + Release Timeline | 322줄 / 9733B  | PASS (warning) |

`CHANGELOG.md`는 **1320줄 / 63019B → 559줄 / 38106B** (prose 498줄). 원래 기준
(46080B, prose 500줄)을 그대로 만족한다.

백로그는 실측 결과 68건이 아니라 **71건**이었다(`c3231f7..HEAD`의 feat/fix/perf/refactor).
주제별로 묶어 `[Unreleased]` 최상단에 Added 9항목 / Fixed 9항목으로 서술했다. 커밋 이력에서
역구성한 것이라는 사실을 파일 안에 주석으로 남겼다 — 각 diff를 읽고 쓴 항목이 아니다.

### 밀도 규칙과 예산

헤더에 항목당 3~5줄, 관련 커밋은 한 항목으로 묶기, 기계 목록은 goreleaser 소관이라는 규칙을
명시했다. 여기에 더해 파일 첫 줄에 예산을 선언했다:

```markdown
<!-- size-limit: 700 -->
```

이 마커는 기본 규칙을 대체하며 **prose줄이 아니라 전체 줄**로 센다(실측: 100으로 낮춰
`File length 558 lines exceeds custom limit 100 lines` / exit 1 확인, 다시 700으로 복구).
현재 559줄이므로 여유 141줄이다.

면제(`exempt`)가 아니라 예산으로 잡은 이유: 63KB까지 자란 원인이 압력 부재였다. 체인지로그는
앞에서부터 읽는 가이드가 아니라 누적 기록이라 기본 500줄 예산이 장르에 맞지 않지만, 상한
자체가 없으면 같은 일이 반복된다.

#### 2026-08-25 정정 — 예산 700 → 1000

700은 근거 없이 고른 값이었고 실측과 충돌했다. 이번 배치가 78항목/460줄(평균 5.9, 중앙값 4)
이므로 여유 141줄은 한 배치의 1/3이다. 즉 다음 배치를 쓰다가 **중간에** 막히는 값이었고,
그때의 해제 조치(릴리스를 끊어 `docs/changelog/`로 옮기기)는 막힌 바로 그 파일을 편집하는
일이라 우회가 필요했다.

1000으로 올려 한 배치가 통째로 들어갈 여유(447줄)를 뒀다. 상한에 닿는다는 것은 이제
"이 파일을 줄여라"가 아니라 "릴리스가 밀렸다"는 신호다. 예산 성격은 그대로 — 초과는 여전히
error이고 편집이 막힌다(500으로 낮춰 `File length 563 lines exceeds custom limit 500 lines`
확인, 다시 1000으로 복구).

### 부수 효과: mdformat 드리프트 정리

`CHANGELOG.md`는 HEAD 시점에 이미 `mdformat --check`를 통과하지 못했다. 크기 게이트에
막혀 아무도 편집하지 못하니 포매터도 한 번도 돌지 않은 것이다. 이번 편집으로
`make format-md-diff`가 처음 적용되면서 세 종류가 정규화됐다:

- 인라인 코드 안에서 끊겨 있던 줄이 합쳐졌다(코드 스팬 내부는 줄바꿈이 불가능하다).
- 구분선 `---`가 파일의 나머지와 같은 `______` 스타일로 통일됐다.
- `` ` -> ` `` 가 `` `->` `` 로 줄었다. CommonMark가 코드 스팬 양끝 공백 한 칸을 원래
  제거하므로 **렌더링 결과는 바뀌지 않는다** — 소스가 렌더 결과를 따라간 것이다.

원본 `[Unreleased]` 본문과 과거 릴리스 본문은 이 정규화 외에 변경이 없음을 기계적으로
확인했다(과거 릴리스 605개 비공백 줄 전수 비교 결과 identical).

### 남은 구조적 사실 (Open Question 답)

`[Unreleased]`가 445줄까지 자란 것은 **릴리스 주기 문제가 맞다.** 태그가 하나도 없고
(`git tag`가 빈 목록) `VERSION`은 0.7.0에 멈춰 있어, 릴리스되지 않은 작업 배치가 두 묶음
쌓여 있었다. 여기에 71커밋 백로그가 세 번째로 얹혔다.

분할과 예산으로 지금 당장의 차단은 풀렸지만, 지속 가능한 해법은 **릴리스를 끊는 것**이다.
태그 생성과 `VERSION` 변경은 메인테이너 결정이라 이 태스크 범위 밖이며(범위 경계 참조),
그 판단이 필요하다는 사실을 헤더 규칙에도 적어 두었다: 예산에 닿으면 릴리스를 끊고 해당
라인을 `docs/changelog/`로 옮긴다.
