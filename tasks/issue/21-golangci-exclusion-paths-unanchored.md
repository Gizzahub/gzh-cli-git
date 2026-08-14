# ISSUE: `.golangci.yml` 제외 패턴이 앵커 없는 정규식이라 `cmd/` 전체가 린트된 적이 없다

- status: open
- priority: P1
- category: build / quality gate
- created_at: 2026-08-13T00:00:00+09:00
- affects: `cmd/` 트리 전체(린트 미적용), `branch-ready.sh` 비악화 판정
- spawned_from: `feat/info-columns` 통합 중 게이트가 "이 브랜치가 도입한 실패"로 오판한 원인 추적

## 요약

`exclusions.paths`의 항목은 **glob이 아니라 정규식**이다. 세 항목이 앵커 없이 들어가 있다.

```yaml
# .golangci.yml:389-400
    paths:
      ...
      - vendor
      - .git      # ← `.` 는 임의의 한 글자. "z-git", "-git", "/git" 전부 매치
      - tmp       # ← 경로 어디에 있든 "tmp" 세 글자면 매치
```

결과가 두 가지다. 하나는 지금 살아 있고, 하나는 잠복해 있다.

## 결함 1 — `cmd/gz-git/` 이 `.git` 패턴에 걸려 통째로 제외된다 (live)

모듈 바이너리 이름이 `gz-git`이라 **모든 `cmd/gz-git/...` 경로가 `.git` 정규식에 매치된다.**

```console
$ golangci-lint run -c .golangci.yml -v ./cmd/...
[runner/exclusion_paths] Skipped 293 issues by pattern ".git"
0 issues.

$ golangci-lint run -c .golangci.yml -v ./...
[runner/exclusion_paths] Skipped 404 issues by pattern ".git"
[runner/exclusion_paths] Skipped 0 issues by pattern "tmp"
0 issues.
```

즉 `make lint`가 초록이라는 사실은 **`cmd/` 에 대해 아무것도 말해주지 않는다.**
CLI 명령 구현 전체가 린트 없이 누적돼 왔다.

`.git` 을 제외하려는 원래 의도(`.git` 디렉터리 스킵)는 애초에 불필요하다 —
golangci-lint는 Go 패키지 목록을 대상으로 돌지 디렉터리를 훑지 않는다.

## 결함 2 — `tmp` 패턴이 게이트의 기준선을 무력화한다 (dormant)

`branch-ready.sh`는 "원래부터 실패였다"를 주장이 아니라 관측으로 확인한다. 대상 tip을
**`mktemp -d` 로 만든 워크트리**에 체크아웃해 같은 명령을 돌린다.

```bash
# ~/devenv/scripts/branch-ready.sh:177,186,188-190
baseline_root="$(mktemp -d)"                  # → /var/folders/.../T/tmp.XXXXXXXX
base_out="$(cd "$baseline_root/wt" && ... make "$target" 2>&1)"
if [ "$base_rc" -eq 0 ]; then
    fail "... 대상 tip에서는 통과. 이 브랜치가 도입한 실패임"
```

`mktemp -d`가 만드는 디렉터리 이름은 항상 `tmp.XXXXXXXX`다. 앵커 없는 `tmp` 패턴이
그 경로에 매치되어 **기준선에서는 모든 진단이 버려지고 rc=0** 이 된다.

그래서 master가 린트 지적을 갖고 있는 동안에는, 그 지적을 그대로 물려받은 브랜치가
전부 "이 브랜치가 도입한 실패"로 판정된다. 실측: `feat/info-columns`가 자기가 건드리지
않은 5개 파일의 지적 20건 때문에 통합 불가였다.

커밋 `068022e`로 그 20건을 없애 master가 0건이 되면서 **현재는 잠복 상태**다.
master에 지적이 하나라도 다시 생기는 순간 같은 방식으로 재발한다.

## 앵커를 붙이면 드러나는 양

```yaml
      - (^|/)vendor/
      - (^|/)[.]git(/|$)
      - (^|/)tmp/
```

```console
$ golangci-lint run -c <anchored-and-unlimited> ./... # master 326f6e4 기준
254 issues:
* godot: 143      * gocritic: 30   * staticcheck: 26  * noctx: 13
* misspell: 11    * errcheck: 5    * gosec: 4         * unparam: 4
* gocyclo: 3      * govet: 3       * nilerr: 3         * thelper: 3
* errorlint: 2    * tagliatelle: 2 * unconvert: 2
```

기존의 **71은 하한**이었다. `.golangci.yml`의 `max-issues-per-linter: 10`에 여러
린터가 걸려 그 뒤는 보고조차 되지 않았다. 상한을 풀어 다시 측정한 실제 총량은
254건이다. 이 중 `godot` 143건과 `misspell` 11건, 합계 154건(61%)은 `--fix`로
자동 수정할 수 있다.

`godot`을 제외한 111건의 디렉터리 분포는 `cmd/` 77건, `pkg/` 28건,
`internal/` 6건이다. 상위 파일은 `watch.go` 13건, `clone_test.go` 11건,
`clone_config.go` 11건, `executor_git_test.go` 8건, `commit.go` 7건이다.

## Acceptance Criteria

- [ ] 세 패턴에 앵커를 붙인다 (`vendor`, `.git`, `tmp`)
- [ ] `golangci-lint run -v ./cmd/...` 의 `Skipped ... by pattern ".git"` 가 0이 된다
- [ ] `max-issues-per-linter` / `max-same-issues` 를 일시적으로 올려 **실제 총량**을 먼저 측정하고
      이 문서에 기록한다 (71은 상한에 잘린 값)
- [ ] 측정된 지적을 정리한다. 한 커밋에 다 넣지 않는다 — 린터별 또는 패키지별로 쪼갠다
- [ ] `make lint` 가 앵커 적용 상태에서 0건으로 완주한다
- [ ] `.git` 제외 항목이 필요한지 재검토한다. Go 패키지 목록 기반이라 불필요할 가능성이 높다

## 순서 주의

앵커링과 지적 정리는 **같은 브랜치에 있어야 한다.** 앵커만 먼저 통합하려 하면
게이트가 막는다: 브랜치는 71건으로 실패하고, 기준선은 master의 옛 설정으로 돌아
0건 통과하므로 "이 브랜치가 도입한 실패"가 된다. 기준선은 브랜치의 설정 파일을
읽지 않는다.

## 범위 경계

- `branch-ready.sh` 는 이 저장소 소유가 아니다. 여기서 고치는 것은 `.golangci.yml` 뿐이다.
  다만 `mktemp` 경로에 걸리는 패턴을 두면 안 된다는 제약은 이 저장소가 지켜야 한다.
- `pkg/handoff` 의 tracked/untracked 계산 결함은 `068022e` 직전 커밋 `8db79dd`에서
  이미 처리됐다. 남은 미결은 "untracked만 있는 저장소를 handoff 차단 사유로 볼 것인가"
  하나이고, 그것은 이 태스크가 아니다.

## References

- `.golangci.yml:389-403` — `exclusions.paths`, `issues.max-issues-per-linter`
- `~/devenv/scripts/branch-ready.sh:172-237` — `baseline_verdict()`
- `068022e` — 게이트를 다시 비교 가능하게 만든 20건 정리 (커밋 메시지에 같은 원인 기록)
- 관련: `18-golangci-v1-binary-breaks-v2-config-lint-gate.md` (같은 게이트가 다른 이유로 내려가 있던 건)
