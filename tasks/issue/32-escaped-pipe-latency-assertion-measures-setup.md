# ISSUE: escaped-pipe 대기 단언이 setup 시간까지 재서 부하에 따라 실패

- status: done
- priority: P1
- category: quality/test-determinism
- created_at: 2026-09-02T21:30:00+09:00
- affects: `pkg/integrate/readiness_waitdelay_unix_test.go` `TestExecuteReadinessWaitDelayBoundsEscapedPipe` (darwin/linux)
- findings: `latency-assertion-spans-unrelated-setup`,
  `setup-deadline-tuned-as-if-it-were-the-assertion`
- related: [31-hosted-only-test-failures-shell-and-default-branch.md](31-hosted-only-test-failures-shell-and-default-branch.md)

## Background

이슈 31로 hosted `Quality gate`가 master에서 초록이 된 직후, 문서 전용 브랜치(설계 문서 130줄

- 카드 46줄, 코드 변경 0줄)에서 canonical 게이트가 이 테스트 하나로 실패했다. 변경과 무관한
  실패였으므로 master 기준선을 측정했다.

| 대상                         | 실행           | 결과                      |
| ---------------------------- | -------------- | ------------------------- |
| master `42773b7`             | `-count=5` × 5 | 1회 실패 (약 20회 중 1회) |
| 문서 브랜치 (docs-only diff) | `-count=5` × 3 | 2회 실패, 1회 통과        |

즉 **선재 결함**이며 특정 브랜치와 무관하다. 실패 모드가 두 가지로 관측된 점이 단서였다.

- `readiness_waitdelay_unix_test.go:42: escaped helper did not publish pid` (2.00s)
- `readiness_waitdelay_unix_test.go:49: escaped pipe exceeded bounded wait: 3.504659375s`

## Root cause

`started`가 측정 대상보다 앞에서 찍혔다. 단언이 주장하는 구간과 실제로 재는 구간이 다르다.

```text
started := time.Now()        ← 시계 시작
go executeReadinessWithTimeout(...)   ← /bin/sh 실행 → 테스트 바이너리 re-exec
waitForReadinessPID(...)     ← PID 파일 폴링 (데드라인 2s)
cancel()                     ← 단언이 말하는 구간은 여기서 시작
elapsed > readinessWaitDelay + time.Second   ← 예산 3s
```

테스트가 주장하는 것은 "취소 이후, escaped 프로세스가 파이프를 붙잡고 있어도 `readinessWaitDelay`
안에 반환한다"이다. 그런데 `elapsed`에는 러너 spawn과 헬퍼 re-exec 시간이 포함된다.

예산 3s 중 대기 구간이 결정적으로 2s를 쓰므로 setup에 허용된 여유는 **1s뿐**이다. `/bin/sh`가 Go
테스트 바이너리를 re-exec 하는 비용은 로드에 따라 그 1s를 넘긴다. 정상 구현도 실패하는 단언이다.

두 실패 모드는 같은 원인의 정도 차이다.

- spawn이 1.5s 걸림 → `elapsed` 3.50s → 라인 49
- spawn이 2s를 넘김 → PID 폴링 데드라인 초과 → 라인 42

`waitForReadinessPID`의 2s 데드라인은 setup 대기인데 단언 데드라인처럼 빡빡하게 잡혀 있었다.

## Fix

1. `started`를 `cancel()` 직전으로 이동. 단언이 명명한 구간만 측정한다.
1. `waitForReadinessPID` 데드라인 2s → 30s. setup 대기는 넉넉해도 무방하며, 헬퍼가 아예 뜨지
   않으면 여전히 실패한다 — 늦게 실패할 뿐이다.

단언 바운드(`readinessWaitDelay + time.Second`)는 **그대로 두었다**. 여유를 늘려 무마한 것이
아니라 잡음을 측정 대상에서 제거한 것이므로, 회귀 탐지력은 오히려 올라간다. 이전에는 1s 여유를
spawn이 잠식했고 이제는 대기 메커니즘에만 쓰인다.

## Acceptance

- [x] 수정 후 반복 실행에서 실패 0건 — `-count=10` × 6배치 = **60/60 통과**, 전 배치 exit 0
  (수정 전 기준선은 약 20회 중 1회 실패)
- [x] 단언이 공허해지지 않았음을 음성 대조군으로 확인 — 바운드를 임시로 `time.Millisecond`로
  조이면 3/3 실패하며 측정값은 `2.001136041s` / `2.00060925s` / `2.001175917s`.
  정확히 `readinessWaitDelay`(2s) + 약 1ms 지터로, 측정 대상이 대기 구간 하나임이 확인된다.
  여유 1000ms 대비 분산 1ms이므로 마진은 약 1000배. 대조군 변경은 커밋하지 않았다
- [x] canonical `GOWORK=off make quality-check`가 exit 0
