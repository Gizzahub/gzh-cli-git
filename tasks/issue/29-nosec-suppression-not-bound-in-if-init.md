# ISSUE: `if` init 절의 `#nosec`이 바인딩되지 않아 canonical 게이트가 fail-closed

- status: done (2026-09-02) — master `1b1298e`에 통합, hosted 초록 확인
- priority: P0
- category: quality/security-gate
- created_at: 2026-09-02T15:40:00+09:00
- affects: `deb085b` 이후 전 리비전 (master `f7fb3f3` 포함)
- findings: `nosec-suppression-registered-on-wrong-line`, `defect-specific-to-pinned-gosec-v2.22.10`,
  `master-quality-gate-red-untracked`, `gate-does-not-pin-gosec-on-path`

## Background

저장소의 canonical 게이트 `make quality-check`가 **`security-code` 단계에서 exit 2로 실패**한다.
`.make/quality.mk:256-259`의 `security-code`는 `GOWORK=off gosec ./...`를 예외 목록 없이
fail-closed로 돌리므로, gosec가 발견한 항목은 그대로 게이트 실패가 된다.

`.github/workflows/ci.yml:60`이 동일한 `make quality-check`를 실행하므로 **hosted CI도 같이
빨간불**이다. master 기준 최근 3개 커밋이 연속 실패했다.

| 커밋      | hosted run  | 결과                                                |
| --------- | ----------- | --------------------------------------------------- |
| `f7fb3f3` | 33585705308 | failure — Quality gate / Run canonical quality gate |
| `e709fe8` | 33584679923 | failure — 동일                                      |
| `38f8db0` | 33581193624 | failure — 동일                                      |

이 실패는 이 저장소의 어떤 이슈 카드로도, devbox 태스크 큐로도 추적되지 않고 있었다.
오히려 devbox `tasks/todo/124-gz-git-context-reference-and-hook-wiring.md`는 이 게이트를
"`make quality-check` passed"로 기록해 두어, **사실과 반대로 현행화되어 있었다.**

## Root cause

> 이 절은 최초 작성본에서 틀렸다. 독립 리뷰가 지적했고, 아래는 재측정으로 확정한 내용이다.
> 폐기된 설명은 "주석이 `*ast.IfStmt`에 붙고 억제가 init 절 안쪽 `*ast.CallExpr`까지
> 내려오지 않는다"였다. 그 설명이 맞다면 `IfStmt`는 225-227행을 차지하므로 225행의 지적이
> **덮였어야** 한다 — 관측된 동작과 정반대다.

gosec는 **AST 서브트리가 아니라 라인 범위**로 억제한다. pinned v2.22.10의
`updateIgnoredRulesForNode`(`analyzer.go:699-712`)는 `issue.GetLine(file, n)` — 주석이
연결된 노드 **자신의** 라인 범위 — 를 `context.Ignores`에 등록하고,
`getSuppressionsAtLineInFile`이 지적의 라인을 그 범위와 대조한다. 조상·자손 전파는 없다.

그리고 후행 주석이 연결되는 노드는 `IfStmt`가 아니다. gosec가 쓰는 것과 같은
`ast.NewCommentMap`을 두 트리에 돌린 실측:

```
BASE  (f7fb3f3):  주석 225행 -> *ast.ReturnStmt  226-226행
                  주석 228행 -> *ast.AssignStmt  228-228행
                  주석 233행 -> *ast.ReturnStmt  234-234행
FIXED (ab88ffe):  주석 225행 -> *ast.AssignStmt  225-225행
                  주석 229행 -> *ast.AssignStmt  229-229행
                  주석 234행 -> *ast.AssignStmt  234-234행
```

후행 주석은 `if` 문이 아니라 **그 본문 안의 `return true`** 에 붙는다.

둘을 합치면 실제 메커니즘이 나온다: 225행의 주석이 **226행**에 대한 억제를 등록하는데
G204 지적은 **225행**에 보고되므로 두 범위가 겹치지 않는다. 수정이 통하는 이유도 같다 —
호출을 대입문으로 꺼내면 주석이 **225행 자신을 차지하는** `AssignStmt`에 붙는다.

같은 파일에서 동일한 주석이 정상 동작하는 곳은 전부 한 줄짜리 대입문, 즉 주석이 연결된
노드가 주석과 같은 줄을 차지하는 형태다:

| 위치                                   | 형태                                               | 억제     |
| -------------------------------------- | -------------------------------------------------- | -------- |
| `integration_participation.go:208`     | `output, err := exec.CommandContext(...).Output()` | 적용됨   |
| `integration_participation.go:228`     | `output, err := exec.CommandContext(...).Output()` | 적용됨   |
| `integration_participation.go:253`     | `output, err := exec.CommandContext(...).Output()` | 적용됨   |
| `integration_participation.go:265`     | `cmd := exec.CommandContext(...)`                  | 적용됨   |
| `integration_participation.go:273`     | `cmd := exec.CommandContext(...)`                  | 적용됨   |
| **`integration_participation.go:225`** | `if err := exec.CommandContext(...).Run(); ...`    | **무효** |
| **`integration_participation.go:233`** | `if err := exec.CommandContext(...).Run(); ...`    | **무효** |

즉 작성자는 억제 의도를 명시했으나 문법 형태 때문에 반영되지 않았다. 보안 판단 자체는
타당하다 — `branch`는 `SanitizeBranchName`으로 검증되고, `remote`는 `git remote` 출력에서
오며, 모든 값이 셸을 거치지 않고 argv로 전달된다.

`pkg/config/integration_participation.go`는 `deb085b feat(workspace): reconcile integration participation`에서 추가된 이후 한 번도 수정되지 않았다. 따라서 이 결함은 그 커밋에서 유입됐고
D6(`f7fb3f3`)와는 무관한 선행 결함이다.

### 이 결함은 pinned gosec 버전에 한정된다

수정하지 않은 base 트리에, 같은 패키지 집합으로, 바이너리만 바꿔 실측:

```
$ GOWORK=off gosec ./pkg/config/...            # v2.22.10 (.make/tools.mk:13 pin)
  Files : 15  Lines : 5938  Nosec : 15  Issues : 2   exit 1
$ GOWORK=off <v2.26.1>/gosec ./pkg/config/...  # v2.26.1
  Files : 15  Lines : 5938  Nosec : 15  Issues : 0   exit 0
```

v2.26.1의 `updateIgnoredRulesForNode`(`analyzer.go:907-940`)는 등록 범위를
`min(n.Pos, group.Pos) .. max(n.End, group.End)`로 넓히며, 소스 주석이 그 의도를 밝힌다 —
"This handles cases where the comment is associated with a subsequent node". 상류의 이
변경이 원래의 `if`-init 형태를 동작하게 만든다.

따라서 **`GOSEC_VERSION`을 올리면 이 카드의 전제가 사라진다.** 그렇더라도 이 수정은
유지할 값어치가 있다: 두 버전 모두에서 동작하고, 억제를 새로 만들지 않으며, 파일의 나머지
다섯 곳과 형태가 같아진다. 다만 아래 "그 밖에"의 탐색 항목은 v2.22.10에 머무는 동안에만
의미가 있다.

## Reproduction

```
$ GOWORK=off gosec ./...
[.../pkg/config/integration_participation.go:233] - G204 (CWE-78): Subprocess launched with
    a potential tainted input or cmd arguments (Confidence: HIGH, Severity: MEDIUM)
[.../pkg/config/integration_participation.go:225] - G204 (CWE-78): ... (Confidence: HIGH)
Summary:
  Files  : 279
  Lines  : 67984
  Nosec  : 77
  Issues : 2

$ GOWORK=off make quality-check ; echo $?
make: *** [security-code] Error 1
2
```

## Resolution

억제 범위를 넓히거나 `gosec` 예외 목록을 도입하지 않는다. 두 호출을 이 파일이 이미 쓰고 있는
`cmd := exec.CommandContext(...)` 형태로 문 밖에 꺼내어, 주석이 의도한 노드에 걸리게만 한다.
`*exec.Cmd`를 만든 뒤 `Run()`을 호출하는 것은 기존 동작과 완전히 동일하다.

검증 기준: `GOWORK=off gosec ./...`의 `Nosec` 카운트가 **변하지 않고**(77 유지) `Issues`만
2 → 0이 되어야 한다. 카운트가 늘어나면 새 억제를 추가한 것이므로 잘못된 수정이다.

## Acceptance criteria

- [x] `GOWORK=off gosec ./...` → `Issues : 0`, exit 0, `Nosec : 77` 유지
  verify: `GOWORK=off gosec ./... 2>&1 | grep -q 'Issues : 0'`
  증거: Files 279, Lines 67986, Nosec 77, Issues 0, exit 0 (수정 전 Issues 2)
- [x] `GOWORK=off make quality-check` → exit 0
  verify: `GOWORK=off make quality-check`
  증거: `ab88ffe` 트리에서 2026-09-02 16:00~16:1x 실행, `✅ Canonical quality gate passed!`, exit 0.
  quality-workspace-check / format-check / lint-check / security-code / security-deps /
  quality-build / test-install-audit / test-unit-quality / test-integration-quality /
  test-e2e-only 전 단계 통과
- [x] hosted CI `Quality gate`가 master에서 초록
  — master `1b1298e`, run
  [33606203470](https://github.com/Gizzahub/gzh-cli-gitforge/actions/runs/33606203470)
  `conclusion=success`. 다만 G204 수정 자체는 `fc8db6c`/run `33604081350`에서 이미
  `security-code`를 통과했고, 그때 드러난 후속 실패는 별건인 이슈 31에서 처리했다.
  verify: human — 통합 후 hosted run URL을 기록한다
- [x] 새로운 `#nosec` 주석을 추가하지 않는다 (억제 총량 불변)
  verify: `test "$(git diff origin/master -- '*.go' | grep -c '^+.*#nosec')" = "$(git diff origin/master -- '*.go' | grep -c '^-.*#nosec')"`

## Follow-up

### 이 저장소는 gosec blanket 억제 표면을 기본값에 암묵적으로 의존한다

gzh-cli 쪽 TASK-116이 pinned 바이너리로 실측한 결과, gosec의 blanket 억제 태그는
**`"#" + `.gosec.json`의 `global.nosec` 설정값`** 이다. 키를 켜고 끄는 게 아니라 태그
문자열을 바꾼다.

| `.gosec.json`                                 | 살아 있는 blanket 태그 | `// #nosec` | `//gosec:disable` |
| --------------------------------------------- | ---------------------- | ----------- | ----------------- |
| 키 없음 / `{}` / `-conf` 없음 ← **이 저장소** | `#nosec`               | **억제됨**  | 동작              |
| `{"global":{"nosec":false}}`                  | `#false`               | 보고됨      | 동작              |
| `{"global":{"nosec":true}}`                   | 없음                   | 보고됨      | **깨짐**          |

이 표는 독립 리뷰가 이견을 제기해 v2.22.10 pinned 바이너리로 **재측정했고 그대로 유지된다**
(`#nosec` / `//gosec:disable` / `#false` / `#skipme` 네 형태를 한 트리에 두고 설정별로 어느
라인이 보고되는지 확인). `nosec: true`는 태그를 `#true`로 바꾸는 것이 아니라 억제를 통째로
끄며, `//gosec:disable`도 함께 무력화된다(Nosec 0, 세 지적 전부 보고). `#nosec` 대체 태그
(`global["#nosec"]`)는 기존 `#nosec`을 대체하지 않고 **추가**된다.

이 저장소에는 `.gosec.json`도 `.gosec.yaml`도 없고 `security-code`는 `-conf` 없이
`gosec ./...`만 실행하므로(`.make/quality.mk:259`), 살아 있는 태그가 `#nosec`이고 77건의
기존 억제가 동작한다. 이 카드의 수정도 그 전제 위에 서 있다.

다만 이는 **가족 저장소 간 보안 게이트 자세가 갈라져 있다**는 뜻이기도 하다. gzh-cli는
blanket 표면을 명시적으로 닫고 `//gosec:disable` 등록 문법 위에 레지스트리를 세우는 반면,
이 저장소는 열린 기본값에 기대며 억제 77건이 어디에도 등록되어 있지 않다. 어느 쪽으로
수렴할지는 이 카드의 범위가 아니지만, **결정 전에 이 저장소에 `.gosec.json`을 추가하면
77건이 한꺼번에 되살아나 게이트가 다시 빨간불이 된다** — 특히 "더 엄격해 보인다"는 이유로
`nosec: true`를 넣으면 `//gosec:disable`까지 무력화된다.

### `make fmt`이 canonical 게이트보다 공격적이다 (이 작업 중 발견)

이 카드를 처리하면서 관측한 별개 사실이다. `make fmt`(= `format-simplify`)를 돌리면
이 변경과 무관한 `cmd/gz-git/cmd/integrate_bootstrap.go`, `pkg/integrate/bootstrap.go`,
`pkg/integrate/readiness_update.go` 세 파일의 import 그룹이 재배치된다. 그런데
`format-check`는 `gofumpt -l`과 `goimports -l`만 보므로 이 세 파일을 지적하지 않는다 —
수정 전 트리에서 `format-check`가 통과한 것이 그 증거다.

즉 **저장소가 안내하는 커밋 전 포매터가 게이트가 요구하는 것보다 더 많이 바꾼다.**
관행대로 `make fmt`을 돌리고 커밋하면 무관한 파일이 조용히 딸려 들어간다. 이 카드에서는
세 파일을 되돌려 변경 범위를 유지했다. 어느 쪽이 권위인지 결정하고 둘을 일치시키는 것은
별도 항목으로 다룰 값어치가 있다.

### 게이트가 PATH의 gosec 버전을 고정하지 않는다 (선행 결함)

`.make/quality.mk:258`은 `command -v gosec`만 확인하므로 게이트는 **PATH에서 먼저 잡히는
아무 gosec**로 돈다. `.make/tools.mk:13`의 `v2.22.10` 핀은 설치 경로에만 적용된다.
바로 위에서 측정했듯 새 gosec를 가진 개발자는 **수정되지 않은 트리에서도 `Issues : 0`을**
보게 되므로, 로컬과 hosted CI의 판정이 조용히 갈라진다. 이번 결함이 로컬에서 늦게 발견된
경로이기도 하다. 이 카드에서 고치지 않았다 — 게이트 실행 방식 변경이라 범위가 다르다.

### 그 밖에

v2.22.10에 머무는 동안, 동일한 `if err := exec.Command...(); ` 형태가 다른 파일에 더 있는지
확인한다. 현 시점 저장소 전체 gosec 결과가 0이므로 다른 무효 억제로 가려진 실제 지적은
남아 있지 않다.
