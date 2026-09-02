# ISSUE: `if` init 절의 `#nosec`이 바인딩되지 않아 canonical 게이트가 fail-closed

- status: open
- priority: P0
- category: quality/security-gate
- created_at: 2026-09-02T15:40:00+09:00
- affects: `deb085b` 이후 전 리비전 (master `f7fb3f3` 포함)
- findings: `nosec-comment-binds-to-ifstmt-not-callexpr`, `master-quality-gate-red-untracked`

## Background

저장소의 canonical 게이트 `make quality-check`가 **`security-code` 단계에서 exit 2로 실패**한다.
`.make/quality.mk:256-259`의 `security-code`는 `GOWORK=off gosec ./...`를 예외 목록 없이
fail-closed로 돌리므로, gosec가 발견한 항목은 그대로 게이트 실패가 된다.

`.github/workflows/ci.yml:60`이 동일한 `make quality-check`를 실행하므로 **hosted CI도 같이
빨간불**이다. master 기준 최근 3개 커밋이 연속 실패했다.

| 커밋 | hosted run | 결과 |
|------|-----------|------|
| `f7fb3f3` | 33585705308 | failure — Quality gate / Run canonical quality gate |
| `e709fe8` | 33584679923 | failure — 동일 |
| `38f8db0` | 33581193624 | failure — 동일 |

이 실패는 이 저장소의 어떤 이슈 카드로도, devbox 태스크 큐로도 추적되지 않고 있었다.
오히려 devbox `tasks/todo/124-gz-git-context-reference-and-hook-wiring.md`는 이 게이트를
"`make quality-check` passed"로 기록해 두어, **사실과 반대로 현행화되어 있었다.**

## Root cause

gosec의 `#nosec` 주석은 소스 라인이 아니라 **AST 노드**에 바인딩된다.
주석은 자신을 감싸는 가장 가까운 문(statement) 노드에 붙는데,

```go
if err := exec.CommandContext(...).Run(); err == nil { // #nosec G204
```

에서 그 노드는 `*ast.IfStmt`이고, G204 규칙이 검사하는 `*ast.CallExpr`는 `IfStmt`의
**init 절 안쪽**에 있다. 억제가 호출까지 내려오지 않으므로 주석이 무효화된다.

같은 파일에서 동일한 주석이 정상 동작하는 곳들은 전부 단순 대입문 형태다:

| 위치 | 형태 | 억제 |
|------|------|------|
| `integration_participation.go:208` | `output, err := exec.CommandContext(...).Output()` | 적용됨 |
| `integration_participation.go:228` | `output, err := exec.CommandContext(...).Output()` | 적용됨 |
| `integration_participation.go:253` | `output, err := exec.CommandContext(...).Output()` | 적용됨 |
| `integration_participation.go:265` | `cmd := exec.CommandContext(...)` | 적용됨 |
| `integration_participation.go:273` | `cmd := exec.CommandContext(...)` | 적용됨 |
| **`integration_participation.go:225`** | `if err := exec.CommandContext(...).Run(); ...` | **무효** |
| **`integration_participation.go:233`** | `if err := exec.CommandContext(...).Run(); ...` | **무효** |

즉 작성자는 억제 의도를 명시했으나 문법 형태 때문에 반영되지 않았다. 보안 판단 자체는
타당하다 — `branch`는 `SanitizeBranchName`으로 검증되고, `remote`는 `git remote` 출력에서
오며, 모든 값이 셸을 거치지 않고 argv로 전달된다.

`pkg/config/integration_participation.go`는 `deb085b feat(workspace): reconcile integration
participation`에서 추가된 이후 한 번도 수정되지 않았다. 따라서 이 결함은 그 커밋에서 유입됐고
D6(`f7fb3f3`)와는 무관한 선행 결함이다.

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

- [ ] `GOWORK=off gosec ./...` → `Issues : 0`, exit 0, `Nosec : 77` 유지
      verify: `GOWORK=off gosec ./... 2>&1 | grep -q 'Issues : 0'`
- [ ] `GOWORK=off make quality-check` → exit 0
      verify: `GOWORK=off make quality-check`
- [ ] hosted CI `Quality gate`가 master에서 초록
      verify: human — hosted run URL을 기록한다
- [ ] 새로운 `#nosec` 주석을 추가하지 않는다 (억제 총량 불변)
      verify: `test "$(git diff origin/master -- '*.go' | grep -c '^+.*#nosec')" = "$(git diff origin/master -- '*.go' | grep -c '^-.*#nosec')"`

## Follow-up

동일한 `if err := exec.Command...(); ` 형태가 다른 파일에 더 있는지, 그리고 무효 억제를
조기에 잡을 수 있는지 확인한다. 현 시점 저장소 전체 gosec 결과가 0이므로 다른 무효 억제로
가려진 실제 지적은 남아 있지 않다.
