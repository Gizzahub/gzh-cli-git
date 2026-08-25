# ISSUE: task 브랜치의 upstream이 통합 브랜치를 가리켜도 그렇게 보고되지 않는다

- status: open
- priority: P2
- category: safety / diagnostics
- created_at: 2026-08-25T10:40:00+09:00
- affects: `git worktree add -b <task> <path> origin/<integration>` 및 `git checkout -b <task> origin/<integration>` 로 만든 **모든** task 브랜치
- spawned_from: 2026-08-25 세션에서 서로 다른 두 저장소에서 독립적으로 관측

## 요약

통합 브랜치의 원격추적 ref에서 task 브랜치를 만들면, git이 그 브랜치의 upstream을
**통합 브랜치로** 설정한다. `branch.autoSetupMerge`의 기본값이 `true`이기 때문이고,
이건 오설정이 아니라 git의 정상 동작이다.

문제는 이 상태에 **잘못된 이름이 붙는다**는 것이다. `git branch -vv`를 직접 읽지 않는
한 드러나지 않고, gz-git에서 이 조건에 실제로 걸리는 유일한 자리인 `integrate check`는
그것을 upstream 오지정이 아니라 "아직 push되지 않았다"로 보고한 뒤 `git push`를
처방한다 — 하필 이 상황에서 가장 위험한 명령이다(아래 "gz-git은 무엇을 하고 있나" 절).
그런데 이 상태에서 `git push`가 무엇을 하는지는 사용자의 `push.default` 값에 따라
갈리며, 일부 값에서는 task 브랜치가 그대로 통합 브랜치로 들어간다.

## 관측

같은 날 두 저장소에서 독립적으로 나왔다. 서로 다른 에이전트가 만든 브랜치다.

| 저장소                    | 브랜치                                        | upstream         | 상태       |
| ------------------------- | --------------------------------------------- | ---------------- | ---------- |
| `sigdock-shared`          | `dev/codex/mst/fix/core-contract-consistency` | `origin/master`  | `ahead 11` |
| `familybook-engine-fiber` | `dev/claude/mst/chore/dependency-catchup`     | `origin/develop` | 생성 직후  |

```console
$ git branch -vv
+ dev/codex/mst/fix/core-contract-consistency 3bb7b13 [origin/master: ahead 11] fix(spec): reject unsafe connector X25519 inputs
* master                                      892cae5 [origin/master] feat(spec): add device_id to RevocationNotice
```

`[origin/master: ahead 11]`은 "이 브랜치가 master보다 11개 앞섰다"로 읽히지만, 실제
의미는 "이 브랜치의 **upstream이 master이고** 아직 push되지 않은 커밋이 11개"다. 두
해석은 `git push`가 무엇을 할지에 대해 정반대 결론을 준다.

## 재현

격리된 저장소에서 그대로 재현된다. 특별한 설정이 필요 없다.

```console
$ git checkout -b dev/claude/mst/fix/demo origin/master
$ git branch -vv
* dev/claude/mst/fix/demo 5614a96 [origin/master] init
```

## 실제 위험도는 `push.default`에 달려 있다

여기가 이 이슈의 핵심이고, 처음 추정과 결과가 달랐던 지점이다.
git 2.50.1에서 네 가지 값을 모두 측정했다.

| `push.default`                    | `git push` 결과                                                 |
| --------------------------------- | --------------------------------------------------------------- |
| `simple` (기본값)                 | **거부.** upstream 이름이 브랜치 이름과 달라 push하지 않는다    |
| `upstream`                        | **`dev/claude/mst/fix/demo -> master`.** 통합 브랜치로 들어간다 |
| `tracking` (`upstream`의 구 별칭) | **`dev/claude/mst/fix/demo -> master`.** 동일                   |
| `current`                         | `dev/claude/mst/fix/demo -> dev/claude/mst/fix/demo`. 안전      |
| `matching`                        | 동명 원격 브랜치가 없으므로 no-op                               |

즉 **기본 설정에서는 즉시 사고로 이어지지 않는다.** 위험은 두 경로로 좁혀진다.

**경로 1 — `push.default`를 `upstream`/`tracking`으로 둔 사용자.** 아무 경고 없이
task 브랜치 전체가 통합 브랜치에 얹힌다. 이 값은 "내가 pull하는 곳으로 push한다"는
직관 때문에 의도적으로 설정하는 사람이 적지 않다.

**경로 2 — 기본값 사용자.** git이 거부하면서 출력하는 안내가 문제다.

```console
fatal: The upstream branch of your current branch does not match
the name of your current branch.  To push to the upstream branch
on the remote, use

    git push origin HEAD:master

To push to the branch of the same name on the remote, use

    git push origin HEAD
```

**git 자신이 통합 브랜치로 push하는 명령을 첫 번째 해결책으로 제시한다.** 막힌 상황에서
안내문의 첫 줄을 복사하는 것은 자연스러운 반응이고, 그 결과가 통합 브랜치 직접 push다.
이 경로에서는 도구가 막아준 뒤에 다시 사용자를 밀어 넣는 셈이 된다.

## gz-git은 무엇을 하고 있나 — 침묵이 아니라 오진

이 이슈의 초판은 "gz-git의 어떤 명령도 이것을 보고하지 않는다"고 적었다.
**그 문장은 너무 강했다.** `integrate check`는 이 조건에 걸린다. 다만 다른 이름으로
보고한다.

`pkg/integrate/check.go:212` `checkPushed`는 브랜치의 upstream이 가리키는 SHA와
브랜치 SHA를 비교한다.

```go
if up != plan.BranchSHA {
	return CheckItem{Name: "push", Status: checkFail, Detail: "upstream differs — git push"}
}
```

upstream이 통합 브랜치이면 두 SHA는 당연히 다르다. task 브랜치를 자기 원격 브랜치로
정상 push한 **직후에도** 다르다. 그래서 이 게이트는 반드시 FAIL을 낸다.

이 세션에서 실제로 그렇게 나왔다.

```console
$ gz-git integrate check
  FAIL  push — upstream differs — git push
```

브랜치는 이미 `origin/dev/claude/mst/fix/goreleaser-version-ldflags`에 완전히
올라가 있었다. 실패한 것은 push가 아니라 upstream 지정이었다.

**침묵보다 나쁜 이유가 여기 있다.** 진단이 두 가지를 동시에 틀린다.

1. **원인을 틀린다.** "upstream differs"는 push가 밀렸다는 뜻으로 읽히므로, 사용자는
   upstream 자체를 의심하지 않는다. 이미 push한 사람은 이 실패를 도구 버그로 여기고
   넘긴다 — 실제로 이 세션에서 그렇게 넘어갈 뻔했다.
1. **위험한 교정을 처방한다.** Detail이 제시하는 명령이 `git push`다. 위 표대로
   `push.default=upstream|tracking`이면 이 명령이 정확히 사고를 일으킨다.
   게이트가 사용자를 막아 세운 뒤, 손에 쥐여주는 것이 방아쇠다.

즉 초판이 요구한 "감지"는 이미 있다. **없는 것은 분류다.** 필요한 작업은 감지 로직을
새로 만드는 게 아니라, 같은 조건을 `push` 실패로 접어 넣지 않고 별개의 upstream
오지정으로 갈라내는 것이다.

## 부수 효과 — stranded 계산이 왜곡된다

`ahead N`은 upstream 기준이므로, upstream이 통합 브랜치면 이 숫자는 "push되지 않은
커밋 수"가 아니라 "통합 브랜치와의 차이"가 된다. task 브랜치가 자기 원격 브랜치에
정상적으로 push된 뒤에도 이 값은 0이 되지 않는다. 저장소 상태를 훑는 도구가 이 값을
그대로 읽으면 이미 안전하게 보관된 작업을 유실 위험 항목으로 계속 보고한다.

## 제안

task 브랜치 명명 규약(`dev/{actor}/{host}/{type}/{slug}`)에 부합하는 브랜치의 upstream이
저장소의 통합 브랜치를 가리키면 진단으로 보고한다. 필요한 정보는 이미 다 있다 —
브랜치 이름, upstream, 그리고 `.gz-git.yaml`이 선언한 통합 브랜치.

보고만 하고 **고치지 않는다.** upstream 재설정은 사용자의 push 의도를 바꾸는 일이라
진단이 임의로 할 일이 아니다. 판단 근거를 보여주고 교정 명령을 제시하는 선까지가 적절하다.

```
⚠ dev/claude/mst/chore/dependency-catchup
  upstream이 통합 브랜치(origin/develop)를 가리킨다.
  push.default=upstream|tracking이면 `git push`가 develop으로 들어간다.
  교정: git branch -u origin/dev/claude/mst/chore/dependency-catchup
```

그리고 `integrate check`의 `push` 항목이 이 조건을 삼키지 않게 한다. upstream SHA가
브랜치 SHA와 다를 때, upstream 이름이 통합 브랜치이면 "아직 push 안 됨"이 아니라
upstream 오지정으로 분류하고 `git push`가 아니라 `git branch -u`를 처방한다.
같은 비교를 두 결론으로 나누는 일이라 새 git 호출이 필요 없다.

## 채택하지 않는 대안

**`branch.autoSetupMerge=false`를 권고한다** — 증상이 아니라 편의를 없앤다. 통합
브랜치에서 만든 브랜치의 upstream이 유용한 경우도 있고, 무엇보다 이미 잘못 설정된
기존 브랜치를 고쳐주지 않는다.

**gz-git이 upstream을 자동 교정한다** — 위 "제안"에서 밝힌 이유로 배제한다. 사용자가
의도적으로 그렇게 둔 경우(예: 통합 브랜치를 추적하며 rebase 기준으로 삼는 워크플로)를
말없이 뒤집는다.

**push 시점에 막는다** — gz-git은 `git push`를 가로채지 않는다. 개인 환경의
`guard-git-integration.sh` 훅이 이 역할을 하지만, 훅은 그 환경에만 있고 저장소를
clone한 다른 사람에게는 없다. 진단은 저장소를 따라다녀야 한다.

## 참고

이 이슈는 결함 보고이며 구현 결정은 포함하지 않는다. 다만 초판이 "`info`에 붙일지
별도 서브커맨드로 둘지"를 열어둔 것은 **이미 있는 표면을 몰라서였다.** 착수 시점의
선택지는 그보다 좁다.

- `gz-git info --audit`이 이미 존재하고, finding code 체계를 갖추고 있다
  (`pkg/repository/audit.go:20`). 새 진단은 새 서브커맨드가 아니라 **finding code
  하나 추가**다.
- upstream 관계를 분류하는 자리도 이미 있다 — `evaluateUpstream`
  (`pkg/repository/audit_eval.go:254`). upstream 없음 / 앞섬 / 뒤처짐 / 갈라짐을
  각각 별개 code로 나누고 있으므로, "upstream이 통합 브랜치를 가리킴"은 그 목록에
  자연스럽게 붙는 다섯 번째 상태다.
- 착수 전에 확인할 것: `info`/`info_audit`은 `effective.Branch.DefaultBranch`를
  읽고 `integrate`는 `IntegrationBranch`를 읽는다. **비슷한 개념에 서로 다른 설정
  키를 쓰고 있어서**, 어느 쪽을 "통합 브랜치"의 근거로 삼을지 먼저 정해야 한다.
  둘이 갈리는 저장소에서는 두 명령이 서로 다른 판정을 내게 된다.

## 정정 이력

- 2026-08-25 — "gz-git의 어떤 명령도 이것을 보고하지 않는다"를 정정했다.
  `integrate check`가 이 조건에 걸리지만 `push` 실패로 오분류하고 `git push`를
  처방한다는 사실이 확인되어 "gz-git은 무엇을 하고 있나" 절을
  추가했다. 구현 표면(위 "참고")도 실제 코드 기준으로 좁혔다.
