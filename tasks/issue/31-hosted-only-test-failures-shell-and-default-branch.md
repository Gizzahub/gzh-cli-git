# ISSUE: `pkg/integrate` 픽스처가 로컬 셸·기본 브랜치에 의존해 hosted 게이트에서만 실패

- status: open
- priority: P0
- category: quality/test-portability
- created_at: 2026-09-02T17:05:00+09:00
- affects: `pkg/integrate` 유닛 테스트 (master `fc8db6c`·`f81a8b0` 포함, 도입 시점은 각 테스트 추가 시점)
- findings: `printf-format-carries-undefined-backslash-escape`,
  `fixture-bare-head-follows-init-defaultbranch`,
  `failures-were-masked-by-an-earlier-fail-closed-stage`
- related: [29-nosec-suppression-not-bound-in-if-init.md](29-nosec-suppression-not-bound-in-if-init.md)

## Background

이슈 29의 G204 수정이 master(`fc8db6c`)에 통합되면서 hosted `Quality gate`의 `security-code`
단계가 처음으로 통과했다. 그 결과 게이트가 **처음으로** `test-unit-quality` 단계까지 진행했고,
거기서 4건이 실패했다.

| 커밋      | hosted run  | 실패 단계                                       |
| --------- | ----------- | ----------------------------------------------- |
| `f7fb3f3` | 33585705308 | `security-code` — G204 (이슈 29)                |
| `fc8db6c` | 33604081350 | `test-unit-quality` — `.make/test.mk:33` 에러 1 |

**이 4건은 이슈 29가 만든 회귀가 아니다.** `f7fb3f3..fc8db6c` 디프는
`pkg/config/integration_participation.go`(6줄)와 문서뿐이고, 실패는 전부 `pkg/integrate`에 있다.
앞 단계가 fail-closed로 계속 죽는 동안 뒤 단계는 **한 번도 실행되지 않았고**, 그래서 이 부채가
축적되는 동안 아무 신호도 나오지 않았다. 이슈 29의 수정이 만든 것이 아니라 **드러낸** 것이다.

실패 목록 (run 33604081350):

| 테스트                                                       | 증상                                                     |
| ------------------------------------------------------------ | -------------------------------------------------------- |
| `TestBootstrapRejectsConcurrentTargetAdvance`                | `fatal: a branch named 'master' already exists` (128)    |
| `TestReadinessUpdateRejectsSourceAndTargetRaces/target`      | 동일                                                     |
| `TestCheck_ContractUsesTargetRunnerNotHeadMakefile`          | `runner result is not valid JSON: invalid character '\'` |
| `TestRunChecked_TargetMovedStopsBeforeIntegrationAndReclaim` | 동일                                                     |

네 건 모두 macOS 개발 머신에서는 통과한다. 게이트가 아니라 **테스트가 실행 환경에 의존**한다.

## Root cause

서로 독립된 두 개의 이식성 결함이다.

### A. `printf` 포맷 문자열에 정의되지 않은 `\"` 이스케이프가 들어간다

`pkg/integrate/readiness_test.go`의 readiness 러너 픽스처가 Go 문자열
`"#!/bin/sh\nprintf '{\\\"version\\\":1,...}'\n"`를 파일로 쓴다. 실제 스크립트 내용은:

```sh
#!/bin/sh
printf '{\"version\":1,\"status\":\"ready\",\"summary\":\"ok\"}'
```

POSIX 작은따옴표 안에서 백슬래시는 **리터럴**이다. 따라서 `printf`가 받는 *포맷 문자열*에
`\"`가 그대로 남는다. `\"`는 POSIX가 정의한 이스케이프가 아니어서 처리가 구현마다 갈린다.

측정 (같은 머신, 같은 인자):

| 셸                              | 출력                                            | JSON |
| ------------------------------- | ----------------------------------------------- | ---- |
| `/bin/sh` (macOS = bash 3.2.57) | `{"version":1,"status":"ready","summary":"ok"}` | 유효 |
| `dash`                          | `{\"version\":1}`                               | 무효 |

Ubuntu 러너의 `/bin/sh`는 dash다. 그래서 러너에서만 리터럴 백슬래시가 남고
`invalid character '\'`가 난다. 관측된 오류 문자열과 정확히 일치한다.

작은따옴표 안에서 큰따옴표는 애초에 이스케이프가 **필요 없다**. 백슬래시는 순수한 오류다.

### B. 픽스처의 bare 저장소 HEAD가 `init.defaultBranch`를 따라간다

`bootstrapFixture`(`pkg/integrate/bootstrap_test.go:385-410`)는 `GIT_CONFIG_GLOBAL`을 빈 파일로
덮어쓴 뒤 `git init --bare`를 부른다. 그러면 `init.defaultBranch`가 **git 구현의 내장 기본값**으로
떨어지는데, 이 값이 환경마다 다르다.

| git                      | 빈 global 설정에서 `git init --bare`의 HEAD |
| ------------------------ | ------------------------------------------- |
| Apple Git 2.50.1 (macOS) | `refs/heads/main`                           |
| Ubuntu 러너의 git        | `refs/heads/master` (아래 오류로 역산)      |

픽스처는 `push origin HEAD:master`로 `master`만 만든다. 그러므로:

- macOS: bare HEAD가 존재하지 않는 `main`을 가리켜 두 번째 clone이 unborn `main`에 안착한다 →
  `checkout -b master origin/master`가 **성공**한다.
- 러너: bare HEAD가 실재하는 `master`라 clone이 `master`에 안착한다 →
  `checkout -b master`가 `fatal: a branch named 'master' already exists`로 **실패**한다.

`GIT_CONFIG_SYSTEM`은 픽스처가 덮어쓰지 않으므로, 시스템 설정에 `init.defaultBranch=master`를
주입하면 러너 조건을 로컬에서 그대로 재현할 수 있다. base(`f81a8b0`)에서 재현 확인:

```
--- FAIL: TestBootstrapRejectsConcurrentTargetAdvance (0.75s)
    bootstrap_test.go:247: git [checkout -b master origin/master]: exit status 128
        fatal: a branch named 'master' already exists
--- FAIL: TestReadinessUpdateRejectsSourceAndTargetRaces/target (1.32s)
    readiness_update_test.go:238: 동일
```

## Fix

- A: `readiness_test.go`의 러너 픽스처 4곳에서 불필요한 백슬래시를 제거한다. 결과 스크립트는
  bash와 dash에서 **동일한 바이트**를 출력한다(측정 확인).
- B: 두 호출부(`bootstrap_test.go:247`, `readiness_update_test.go:219`)를 `checkout -b` →
  `checkout -B`로 바꾼다. 테스트의 의도는 "`master`가 없어야 한다"가 아니라 "여기서 `master`가
  `origin/master`를 가리키게 한다"이므로, `-B`가 그 의도를 그대로 진술한다.

## 잔여 (별건)

`bootstrapFixture`의 bare HEAD는 여전히 `init.defaultBranch`를 따라간다. 지금 두 호출부는
`-B`로 무해해졌지만, 앞으로 추가되는 픽스처가 같은 함정을 다시 밟을 수 있다. `git init --bare -b master`로 픽스처를 결정론적으로 고정하는 편이 낫다 — 다만 그러면 로컬 clone이 안착하는 브랜치가
`main`에서 `master`로 바뀌므로 다른 단정에 영향이 없는지 별도로 확인해야 한다. 이번 변경 범위에
섞지 않는다.

## Acceptance

- [x] `GIT_CONFIG_SYSTEM`에 `init.defaultBranch=master`를 준 상태에서 `pkg/integrate` 전체가 통과
  — `ok github.com/gizzahub/gzh-cli-gitforge/pkg/integrate 115.095s` (수정 전 base `f81a8b0`에서는
  같은 조건으로 러너의 4건이 그대로 재현됨)
- [x] 기본(로컬) 상태에서도 `pkg/integrate` 전체가 통과
  — `ok github.com/gizzahub/gzh-cli-gitforge/pkg/integrate 107.067s`
- [x] canonical `make quality-check`가 exit 0
  — `✅ Canonical quality gate passed!` / `QUALITY_EXIT=0` (e2e 포함 10단계 전부). 주의: 같은 머신에서
  다른 golangci-lint가 돌고 있으면 `Error: parallel golangci-lint is running`으로 `lint-check`가
  죽는다(전역 락, 저장소 무관). 게이트는 직렬로 돌려야 한다.
- [ ] hosted `Quality gate`가 master에서 실제로 초록
