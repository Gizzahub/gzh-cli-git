# ISSUE: D6 컨텍스트 참조 계약의 미정의 구간 — W6/W7 착수 전 처리 필요

- status: done (2026-09-02) — P1 4건·P2 4건 문서로 종결, canonical 게이트 exit 0
- priority: P1
- category: architecture/context-reference
- created_at: 2026-09-02T16:10:00+09:00
- affects: `f7fb3f3` (`docs/10-architecture/90-design-decisions.md` §12.1 D6)
- findings: `manifest-state-matrix-undefined`, `componentoutcome-unknown-undefined`,
  `envelope-exit-disagreement-dropped`, `ce-exit-contract-inverts-gz-git`
- blocks: W6 구현 카드, W7

## Background

D6(`f7fb3f3`)에 대해 **독립 리뷰가 처음으로 실제 수행**되었다. 결과는 `0 P0, 4 P1, 4 P2, 4 P3`.

이 커밋은 devbox `tasks/todo/124-gz-git-context-reference-and-hook-wiring.md`와
`tasks/plan/GZ_GIT_CONTEXT_ORCHESTRATION.md`(425행)에 **"독립 리뷰 P0/P1/P2 없음"으로
기록되어 있었으나, 어느 저장소에도 리뷰 산출물이 존재하지 않았다.** 실제로 수행해 보니
P1이 4건 나왔다. 따라서 W5를 "리뷰 클린"으로 취급해서는 안 되며, devbox 쪽 두 기록도
정정되어야 한다(그 편집은 devbox 소유).

D6는 아직 구현이 없다 — `grep -rn "gz-git-context"`는 D6 문서 자신만 매치하고,
`cmd/gz-git/cmd/capability.go`는 `integrate-readiness-v1`,
`integrate-queue-controller-v1`, `integrate-queue-base-missing-v1` 셋만 등록한다.
즉 지금은 **문서만 고치면 되는 시점**이고, W6/W7이 D6에서 코드를 파생시키는 순간
아래 미정의 구간이 그대로 구현 분기로 굳는다.

## P1 findings

### P1-1 매니페스트 자신의 tracked/worktree 상태 매트릭스가 미정의 (`:129-136`)

131행은 매니페스트가 index-tracked여야 한다고 하고 136행은 worktree 바이트를 파싱
소스로 삼는다. 그런데 다음 두 도달 가능한 상태에 결과가 정의되어 있지 않다.

| 상태                                                                    | D6의 정의                                                                                                                                 |
| ----------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| worktree에 있으나 **untracked**                                         | 없음 — 파일이 있으므로 `context.not-declared`도 아니고, 132행의 typed finding은 문법상 "Every entrypoint"에 걸려 매니페스트를 덮지 않는다 |
| tracked이나 **worktree에 없음** (index-only / `skip-worktree` / sparse) | 없음 — 파싱 소스 자체가 존재하지 않는다                                                                                                   |

entrypoint 쪽 상태는 완전히 규정되어 있는 것과 대조된다. 특히 첫 번째 케이스에서
"파일이 있으니 not-declared는 아니고, 그럼 그냥 읽자"가 자연스러운 오답이며,
그렇게 되면 **untracked 매니페스트가 조용히 동작해 tracked-transport 요구가 무력화된다.**

### P1-2 `componentOutcome: unknown`이 어디에도 정의되지 않음 (`:223-231`)

`unknown`은 D6(109-256행) 전체에서 **enum 나열 1회가 유일한 등장**이고, 이를 산출하는
조건이 없다. 또한 D6는 "CE invocation fault"(211행, exit 2)와 "gz-git transport
fault"(195-197행)를 산문에서 구분하면서 토큰은 `fault` 하나만 주고 판별 필드를 두지
않는데, 231행은 두 fault 클래스가 분리 유지되어야 한다고 요구한다. 이 어휘는
workbook 비교와 Sigdock pilot이 소비하므로, 미정의 enum 멤버 + 판별 불가 fault는
저장소별 구현 분기의 전형적 원인이다.

### P1-3 envelope/exit 불일치 규칙이 계획서에서 D6로 넘어오며 누락 (`:194-197`, `:240-244`)

계획서 `GZ_GIT_CONTEXT_ORCHESTRATION.md:150-152`는 "디코딩된 JSON과 모순되는 0/1
프로세스 exit은 CE finding이 아니라 gz-git invocation fault"를 요구하고, 326행은
`envelope/exit disagreement`를 필수 픽스처로 나열한다. D6의 fault 목록에는 start/wait
실패, timeout, cancellation, cap overflow, malformed JSON, capability mismatch,
"unexpected exit"이 있으나 **exit 0 + finding envelope**(또는 exit 1 + pass envelope)는
"unexpected exit"이 아니어서 분류되지 않는다. D6의 픽스처 목록(243행)도 "CE exits
0/1/2"만 적는다. **W7은 계획서가 아니라 D6에서 구현하므로, 요구가 코드가 되는 지점에서
소실된다.**

### P1-4 CE exit 계약이 gz-git 자신의 계약과 1·2에서 의미가 뒤집힘 (`:209-212` vs `pkg/cliutil/exit.go:27-30`)

| 코드 | gz-git (`pkg/cliutil/exit.go`) | CE (D6)              |
| ---- | ------------------------------ | -------------------- |
| 0    | `ExitOK`                       | pass                 |
| 1    | `ExitToolError`                | **provider finding** |
| 2    | `ExitPartialFailed`            | **invocation fault** |
| 3    | `ExitReclaimIncomplete`        | —                    |

D6는 매핑을 정의하지 않는다. 가장 자연스러운 구현인 코드 passthrough는 CE의 *finding*을
gz-git의 *tool error*로, CE의 *프로세스 결함*을 `ExitPartialFailed`(일부 단위가 실패했다는
findings 모양의 의미)로 바꾼다. 이는 **D6가 231행에서 스스로 금지한 "CE 프로세스 결함이
provider finding이 될 수 없다"를 정면으로 위반**한다.

D6의 완화책은 구현 카드가 CE 매핑을 "verbatim" 기록한다는 것뿐(212-214행)이라 바깥으로
나가는 gz-git 코드는 다루지 않는다. D6가 명령 철자를 예약하지 않아도(238-239행)
`cmd/gz-git/cmd/exit_code_test.go:62-78`의 `TestBulkCommandsHaveExitCodesHelp`가 모든 bulk
명령에 "Exit Codes:" 섹션을 강제하므로, 집계 표면에는 반드시 exit 계약이 생긴다.

## P2 findings

- **P2-1 (`:153-162`)** — D6는 매니페스트에 no-follow·component-by-component·fail-closed
  열기를 요구하지만, 이 저장소의 **기존** `.gz-git.*` 탐색은 정반대다.
  `pkg/config/paths.go:190`은 `os.Stat`(심링크 추적)으로 탐침하고,
  `pkg/config/symlink.go`의 `CreateConfigSymlink`는 `.gz-git.yaml`을 **의도적으로
  심링크로 생성**하는 일급 기능이다. 구현자가 형제 파일명이라는 이유로 `pkg/config`
  탐색 헬퍼를 재사용하면 D6 항목 4를 조용히 위반한다. ADR이 "기존 로더는 재사용
  후보가 아니다"를 명시해야 한다.
- **P2-2 (`:161-162`, `:227-228`)** — `unsupported-platform` 토큰이 결과 어휘 절에 없고
  다른 reason code(`context.not-declared`, `gate.not-adopted`)와 달리 점 네임스페이스도
  아니다. 또한 D6는 플랫폼 부재가 실패와 혼동되지 않는다고 하지만, 같은 저장소의 최근접
  유사물인 `pkg/integrate/readiness.go:133`은 Windows에서 그 조건을 `checkFail`로
  보고한다. 저장소 내부에 상충 규약을 도입하면서 그 사실을 언급하지 않는다.
- **P2-3 (`:122`, `:127`)** — "root-local"·"never upward-merged"는 *병합*을 제약할 뿐
  *탐색*을 제약하지 않는다. 부모 디렉터리나 하위 디렉터리의 `.gz-git-context.yaml`이
  무시되는지 finding인지 미정의다. 형제 파일명에 대한 이 저장소의 기존 규약이
  `$HOME`까지 올라가는 upward walk(`DetectConfigFile`)이므로 "올라가서 찾되 병합만 안
  한다"가 문면상 읽히는 해석이다.
- **P2-4 (`:109-256`)** — D1~D5는 Decision/Rationale/Trade-offs 형식의 13~25행인데 D6는
  148행이고 **Rationale과 Alternatives Considered가 없다.** 계획서가 고려했다고 기록한
  두 대안(런타임 네이티브 탐색 추론, guarded apply)을 기각하면서도 기록하지 않았다.
  또 이 문서에 유일하게 존재하는 **Status** 필드를 추가해 "Conditionally accepted"의
  의미가 문서 내에서 정의되지 않는다. `docs/10-architecture/adr/`가 없어 이 무게의
  항목이 갈 곳이 §12.1뿐인 것이 구조적 원인이다.

## P3 findings

- **P3-1** — 소스의 모든 목록 항목이 `1.`이라 237행의 "item 8" 상호참조가 raw 파일에서
  해소되지 않는다(렌더링 시에는 1~9). 앵커나 항목 제목을 쓰는 편이 낫다.
- **P3-2 (`:163-168`)** — 한계값이 직교하지 않는다. 32 entrypoint × 1 MiB = 32 MiB인데
  집계 상한은 4 MiB라, 1 MiB 상한은 4개 이하에서만 도달 가능하고 32개 상한은 사실상
  도달 불가다. 둘이 동시에 걸릴 때 무엇이 권위인지, 집계 상한 초과 시 이미 처리한
  항목을 유지/폐기하는지 명시가 필요하다.
- **P3-3 (`:192-193`)** — "exactly one JSON document plus trailing whitespace"는 후행
  공백을 *요구*하는 것으로 읽힌다. 계획서 표현("only whitespace after it")이 정확하다.
  "plus optional trailing whitespace"로 고친다.
- **P3-4 (`:215-219` vs `:281`)** — 항목 9가 apply를 기각하고 재고에 새 제품 범위 결정을
  요구하는데, 같은 파일 §13.1 "Potential Enhancements (v2.0+)"는 여전히 "Git hooks
  automation"을 상호참조 없이 나열한다. 모순은 아니나 §13.1 독자에게 게이트 신호가 없다.

## 지적이 아닌 관찰 (기록용)

- **잔여 TOCTOU는 은폐가 아니라 명시된 스코프다.** pre/post identity + size + mtime/ctime
  프로토콜(159-161)은 동일 크기·동일 타임스탬프 in-place 재작성을 막지 못하지만, D6는
  서명·의미 무결성 의미를 명시적으로 부인하고(151-152) 픽스처에 "same-size writes"를
  나열한다(241). 스코핑이 정확하다.
- **`git` 실행파일 provenance 비대칭.** D6는 CE에 ambient `PATH` 대신 승인된 디스크립터로
  git을 해석하라고 요구하지만(183-184), gz-git 자신은 `pkg/config/workspace_access.go:163,187`,
  `pkg/integrate/prepare.go:167`, `pkg/reposync/executor_git.go:430` 등에서 ambient `PATH`로
  `git`을 호출한다. 다만 D6는 *새* 신뢰 경계를 규정하는 것이지 기존 코드에 대해 거짓
  주장을 하는 게 아니므로 지적이 아니라 맥락으로 기록한다.
- **provenance 필드 선례가 이미 있는데 인용되지 않았다.** `pkg/integrate/check.go:57-58`,
  `readiness_update.go:40-49`의 `RunnerPath`/`RunnerOID`/`ManifestOID`/`ContractDigest`가
  D6의 entrypoint identity 모델과 가장 가까운 기존 유사물이다. W7은 재발명하지 말고
  재사용해야 한다.

## Acceptance criteria

- [x] P1-1: 매니페스트 상태 매트릭스(untracked / index-only-absent 포함)를 D6에 명시
  — index×worktree 4상태를 표로 정의했다. 핵심은 `componentOutcome`이 aggregation/transport
  만 기술한다는 D6 자신의 규정을 적용해, 네 상태 모두 `observed`이고 상태는 reason code가
  나른다는 점이다. 따라서 저장소 상태 문제가 `fault`로, transport fault가 저장소 상태로
  나타나는 경로가 문법적으로 닫힌다. `untracked | present`는 파싱·다이제스트 모두 금지로
  명시해 리뷰가 지목한 "파일이 있으니 읽자" 오답을 막았다.
  신규 reason code: `context.manifest-untracked`, `context.manifest-not-materialized`
- [x] P1-2: `unknown`의 산출 조건을 정의하거나 enum에서 제거하고, 두 fault 클래스의
  판별 필드를 정의
  — `unknown`을 enum에서 **제거**했다. 산출 조건이 없는 멤버는 저장소별 catch-all이 되며,
  이 어휘가 막으려는 divergence 그 자체다. 향후 멤버 추가는 조건 동시 정의를 요구한다.
  fault 판별은 `faultDomain: gz-git-transport | ce-invocation` 필드를 신설했고,
  `ce-invocation`은 CE exit `2`에서만 설정된다.
- [x] P1-3: envelope/exit 불일치 규칙과 해당 픽스처를 D6에 편입
  — exit `0`+finding envelope와 exit `1`+pass envelope를 transport fault로 명시했다.
  둘 다 계약상 유효한 코드라 "unexpected exit" 절에 걸리지 않는다는 점을 근거로 별도
  명시했고, 한쪽을 신뢰해 해소하지 않고 JSON을 폐기한다는 규칙을 붙였다. 픽스처 목록에
  양방향 불일치를 추가했다.
- [x] P1-4: CE exit code → gz-git exit code 매핑을 D6에 명시하고, CE fault가
  `ExitPartialFailed`로 새지 않음을 보장
  — passthrough를 범주 오류로 규정하고 매핑표를 고정했다. CE `0`/`1` → `0 ExitOK`
  (finding은 payload이지 gz-git 실패가 아니다), CE `2`·기타·envelope 불일치 →
  `1 ExitToolError`. **이 표면은 `2 ExitPartialFailed`와 `3 ExitReclaimIncomplete`를
  결코 산출하지 않는다**고 명시했다. 호출자는 exit이 아니라 JSON으로 pass/finding을
  구분한다.
- [x] P2 4건 처리 또는 명시적 기각 사유 기록
  — P2-1: 기존 `.gz-git.*` 로더(`pkg/config/paths.go`의 `os.Stat`, `pkg/config/symlink.go`의
  의도적 심링크 생성)를 **재사용 후보 아님**으로 명시. P2-2: `unsupported-platform` →
  `context.unsupported-platform`으로 네임스페이스화하고, `pkg/integrate/readiness.go:133`의
  `checkFail` 규약과 갈리는 이유(readiness는 "준비됐나"에 답하므로 미답이 실패, observation은
  미답이 부재)를 기록하며 양쪽 모두 서로 맞추지 않는다고 확정. P2-3: root-local이 병합이
  아니라 **탐색**을 제약함을 명시하고, 부모/하위 디렉터리 파일은 읽지도 보고하지도 않으며
  `DetectConfigFile`의 upward walk는 선례가 아님을 기록. P2-4: Rationale과
  Alternatives Considered(네이티브 탐색 추론·guarded apply·기존 로더 재사용·exit
  passthrough 4건 기각)를 추가하고 **Status "Conditionally accepted"의 의미를 정의**.
- [x] P3 처리 범위 결정
  — P3-3(후행 공백 표현)과 P3-4(§13.1 "Git hooks automation"에 D6 item 9 게이트 상호참조)는
  단문 수정이라 함께 처리했다. **P3-1**(항목 번호 상호참조)과 **P3-2**(32×1 MiB와 4 MiB
  집계 상한의 비직교성)는 이 카드에서 처리하지 않는다. P3-2는 한계값의 권위와 초과 시
  부분 결과 처리라는 **새 설계 결정**이라 W6 구현 카드에서 실측 corpus와 함께 정하는 것이
  맞고, 지금 문서에서 임의로 고정하면 pilot이 측정하기로 한 값을 선점한다.
- [ ] devbox `tasks/todo/124-…md`와 `tasks/plan/GZ_GIT_CONTEXT_ORCHESTRATION.md:425`의
  "독립 리뷰 P0/P1/P2 없음" 기록 정정 (devbox 소유 — 이 저장소 밖)
  verify: human — devbox 측 커밋을 참조로 남긴다
  현황(2026-09-02 확인): 정정문 자체는 작성돼 있고 내용도 맞다 — devbox 브랜치
  `dev/claude/mbp/docs/verify-binding-repair` `50e0b93`이 todo/124에 "4 P1 / 0 P0
  (gzh-cli-gitforge `85bbded`, issue 30)"와 CE 쪽 "3 P1 / 0 P0 (ce-agent-kit
  `bb51745d`, TASK-087)"를 담고 있다. 그러나 **미통합**이라 devbox master `8f40ee6`에는
  반영되지 않았고, plan `422`·`425`행과 todo/124 `95`행은 여전히 거짓 기록을 들고 있다.
  통합을 막는 것은 이 카드와 무관한 devbox 구조 문제(TASK-132)다 — devbox `.make/quality.mk`가
  `cd gzh-cli`를 하는데 하위 저장소가 gitignore 대상이라 워크트리에 존재하지 않아 게이트가
  워크트리에서 성립하지 않는다. 따라서 이 항목은 gitforge 쪽에서 닫을 수 없다
- [x] `GOWORK=off make quality-check` exit 0 — ✅ Canonical quality gate passed! /
  QUALITY_EXIT=0 (e2e 포함 10단계 전부). 이 브랜치는 `b25e2b1`(이슈 32) 위로 리베이스한
  상태에서 측정했다. 그 이전에는 선재 flake
  `TestExecuteReadinessWaitDelayBoundsEscapedPipe`가 문서 전용 diff에서도 게이트를
  붉게 만들어 이 항목을 닫을 수 없었다
  verify: `GOWORK=off make quality-check`
