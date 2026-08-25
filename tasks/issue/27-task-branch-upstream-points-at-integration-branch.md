# ISSUE: task 브랜치의 upstream이 통합 브랜치를 가리켜도 아무도 알려주지 않는다

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

문제는 이 상태가 **보이지 않는다**는 것이다. `git branch -vv`를 직접 읽지 않는 한
드러나지 않고, gz-git의 어떤 명령도 이것을 보고하지 않는다. 그런데 이 상태에서
`git push`가 무엇을 하는지는 사용자의 `push.default` 값에 따라 갈리며, 일부 값에서는
task 브랜치가 그대로 통합 브랜치로 들어간다.

## 관측

같은 날 두 저장소에서 독립적으로 나왔다. 서로 다른 에이전트가 만든 브랜치다.

| 저장소 | 브랜치 | upstream | 상태 |
| --- | --- | --- | --- |
| `sigdock-shared` | `dev/codex/mst/fix/core-contract-consistency` | `origin/master` | `ahead 11` |
| `familybook-engine-fiber` | `dev/claude/mst/chore/dependency-catchup` | `origin/develop` | 생성 직후 |

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

| `push.default` | `git push` 결과 |
| --- | --- |
| `simple` (기본값) | **거부.** upstream 이름이 브랜치 이름과 달라 push하지 않는다 |
| `upstream` | **`dev/claude/mst/fix/demo -> master`.** 통합 브랜치로 들어간다 |
| `tracking` (`upstream`의 구 별칭) | **`dev/claude/mst/fix/demo -> master`.** 동일 |
| `current` | `dev/claude/mst/fix/demo -> dev/claude/mst/fix/demo`. 안전 |
| `matching` | 동명 원격 브랜치가 없으므로 no-op |

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

이 이슈는 결함 보고이며 구현 결정은 포함하지 않는다. 진단을 `info`에 붙일지 별도
서브커맨드로 둘지는 착수 시점에 정한다.
