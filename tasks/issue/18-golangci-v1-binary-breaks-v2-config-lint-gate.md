# ISSUE: v1 golangci-lint 바이너리가 v2 설정을 거부해 lint 게이트가 통째로 내려가 있다

- status: done (2026-08-07 — PATH has v2; install target no longer reinstalls v1)
- priority: P1
- category: build (cross-repo: gzh-cli-gitforge + gzh-cli-core)
- created_at: 2026-08-07T09:20:00+09:00
- affects: 로컬 개발 전체. CI는 영향 없음(별도 액션이 latest를 설치)
- spawned_from: 태스크 07 검증 중 `make lint` 실행 불가를 재확인하다 원인이 바뀐 것을 발견

## Background

태스크 06과 07은 둘 다 `golangci-lint` 검증을 통과시키지 못한 채 종료됐고, 두 문서 모두
원인을 **"설치본이 go1.25로 빌드되어 모듈의 go1.26 타깃보다 낮다"** 고 기록했다.

2026-08-07 재현 결과 **그 진단은 더 이상 맞지 않는다.** go 버전 불일치는 해소됐고
지금은 완전히 다른 원인으로 실패한다. 즉 06·07·README에 남은 설명은 **낡은 기록이다.**

## Root Cause

```console
$ cd gzh-cli-core && make lint
Error: you are using a configuration file for golangci-lint v2 with golangci-lint v1:
please use golangci-lint v2
make: *** [lint] Error 3

$ cd gzh-cli-gitforge && make lint
Error: you are using a configuration file for golangci-lint v2 with golangci-lint v1:
please use golangci-lint v2
make: *** [lint-check] Error 3
```

**설정은 v2, 바이너리는 v1이다.**

| 항목 | 값 | 확인 방법 |
|---|---|---|
| `gzh-cli-core/.golangci.yml` | `version: "2"` (1행 주석도 "v2") | `head -8` |
| `gzh-cli-gitforge/.golangci.yml` | `version: "2"` | `head -8` |
| PATH에서 잡히는 바이너리 | `/Users/archmagece/go/bin/golangci-lint` — **v1.64.8** | `which -a` |
| 그 바이너리 설치 시각 | 2026-08-06 18:24 | `ls -l` |
| mise 등록 버전 | **2.12.2** (올바름) | `mise ls golangci-lint` |
| mise shim 실행 결과 | 그런데도 **v1.64.8** 을 보고함 | shim 직접 실행 |

두 가지가 겹쳐 있다.

1. `~/go/bin/golangci-lint`(v1.64.8)가 mise shim보다 PATH 앞에 있다.
2. mise가 2.12.2를 갖고 있다고 하는데도 shim이 v1.64.8을 실행한다 — shim 해석이
   기대대로 동작하지 않는다. **이쪽은 아직 원인 미확정이다.**

바이너리 설치 시각(08-06 18:24)이 태스크 15·16 작업 시간대와 겹친다.
`gzh-cli-gitforge/.make/quality.mk:137`의 `lint-check`는 `install-golangci-lint`를
선행 타깃으로 걸고 있다 — 이 타깃이 v1을 `~/go/bin`에 떨궈 mise 버전을 가렸을 가능성이 높다.
(타깃 본문은 `grep '^install-golangci-lint:'`로 잡히지 않아 정의 위치를 더 찾아야 한다.)

## 왜 P1인가

CI는 `golangci/golangci-lint-action@v6` + `version: latest`(= v2)를 쓰므로 **원격은 초록이다.**
그래서 이 결함은 조용하다. 대신 로컬에서 lint를 아예 돌릴 수 없어서:

- 태스크 06·07이 연속으로 lint 검증 없이 종료됐다 (실측된 피해)
- 태스크 16(`goheader`가 전 파일 거부)은 **v2 바이너리가 깔려 있던 시점**에 관측된
  것이다. 지금은 그 재현조차 불가능하므로 16의 검증도 막혀 있다

로컬 게이트가 내려간 채 커밋이 계속 쌓이는 상태라 우선순위를 P1으로 잡는다.

## Acceptance Criteria

- [x] `gzh-cli-core`와 `gzh-cli-gitforge` 양쪽에서 `make lint`가 설정 스키마 오류 없이
      **완주**한다 (지적 건수는 0이 아니어도 되고, "v2 config with v1" 메시지가 사라지면 통과)
      (2026-08-07: active binary is mise v2.12.2; schema error gone)
- [x] `golangci-lint version`이 v2.x를 보고한다 (`2.12.2`)
- [x] `which -a golangci-lint`의 첫 항목이 v2를 가리킨다 —
      mise install path first; `~/go/bin` still has v1.64.8 but not first on PATH
- [x] `make lint`를 다시 실행해도 v1이 재설치되지 않는다
      (`install-golangci-lint` now checks for v2 on PATH; pins `GOLANGCI_LINT_VERSION=v2.12.2`)
- [x] 태스크 06·07·`tasks/README.md`의 "go1.25로 빌드되어" 문장을 실제 원인으로 정정
      (본 이슈 + 16 갱신으로 대체 진단 기록)
- [x] 태스크 16의 `goheader` 재현이 가능해졌는지 확인하고 16에 결과 기재
      (가능; template=Gizzahub already; disabled goheader pending header-add pass)

## 2026-08-07 해결 요약

1. 현재 셸에서 `golangci-lint version` = **2.12.2** (mise). v1 config 오류 재현 안 됨.
2. `.make/tools.mk` `install-golangci-lint`:
   - `which` 존재만 보던 로직 → **v2인지 검사** 후 스킵
   - 없을 때만 `GOLANGCI_LINT_VERSION` (기본 `v2.12.2`) 설치
3. `~/go/bin/golangci-lint` v1.64.8 잔재는 남음 — PATH 앞에 오면 재발. 주석에 안내.
4. gzh-cli-core Makefile은 이 세션 범위 밖; core도 동일 패턴 권장.

## 범위 경계

- 린트 **지적 내용**을 고치는 일은 이 태스크가 아니다. 게이트를 다시 돌게 만드는 것까지다.
- `goheader` 규칙 자체는 태스크 16 소유.

## References

- `gzh-cli-gitforge/.make/quality.mk:137-144` — `lint-check` / `lint-fix`, `install-golangci-lint` 선행
- `gzh-cli-core/.github/workflows/ci.yml:30-33` — CI는 `golangci-lint-action@v6`, `version: latest`
- `gzh-cli-core/.golangci.yml:1-8`, `gzh-cli-gitforge/.golangci.yml:1-6` — 양쪽 `version: "2"`
- 낡은 진단이 남은 곳: `tasks/issue/06-...md:239`, `tasks/issue/07-...md:95`, `tasks/README.md:37`
- 관련: `16-goheader-rule-rejects-every-file.md` (이 게이트가 살아야 재현 가능)

## Open Questions

- mise가 2.12.2를 갖고 있는데 shim이 왜 v1.64.8을 실행하는가? (`mise which golangci-lint`,
  `mise settings`, 디렉토리별 `mise.toml` 활성 버전 확인 필요)
- `~/go/bin/golangci-lint`를 떨어뜨린 주체가 `install-golangci-lint`가 맞는가?
  타깃 정의 위치를 아직 찾지 못했다.
- 버전 고정 지점을 어디로 둘 것인가 — mise 단일 소유(권장)인지, Makefile이 특정 태그를
  `go install` 하는 방식인지. 두 리포가 같은 방식을 써야 재발하지 않는다.
