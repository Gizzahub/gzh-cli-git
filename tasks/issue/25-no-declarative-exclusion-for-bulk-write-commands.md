# ISSUE: bulk write 명령을 저장소 단위로 막을 선언형 수단이 없다

- status: done (2026-08-25)
- priority: P2
- category: config / safety
- created_at: 2026-08-25T00:20:00+09:00
- affects: `push`, `commit`, `clean`, `cleanup branch`, `stash`, `tag create`, `pr create`, `switch`, `update`, `exec` — 매 실행마다 `--exclude`를 기억해야만 특정 저장소를 건드리지 않는다
- spawned_from: flow-taskchain-devbox의 `tasuku-repo` 제외 처리를 검토하다 발견
- resolved_by: A(`defaults.scan.exclude`) + C(문서 범위 명시). B(저장소 단위 readOnly)는 채택하지 않음

## 요약

읽기만 하고 절대 쓰지 않아야 할 저장소(vendored 사본, upstream 미러, 남의 저장소를
참조용으로 clone해 둔 디렉터리)를 bulk write 대상에서 **영구히** 빼 둘 방법이 없다.
현재 방법은 매 실행마다 플래그를 붙이는 것뿐이다.

```console
$ gz-git push --exclude tasuku-repo   # 이번 한 번만 안전
$ gz-git push                          # 플래그를 잊으면 그대로 대상에 들어간다
```

실패는 조용하다. 제외를 잊었다는 경고가 없고, 그 저장소가 대상에 포함됐다는 사실은
결과 표에 성공 행으로만 나타난다.

## 사실관계

### 1. bulk 명령은 설정 파일이 아니라 디렉터리를 스캔한다

모든 bulk 명령이 `ScanRepositories`에 `IncludePattern`/`ExcludePattern`을 넘기는데,
그 값의 출처는 **오직 해당 실행의 플래그**다.

```go
// cmd/gz-git/cmd/push.go:144
IncludeSubmodules: pushFlags.IncludeSubmodules,
IncludePattern:    pushFlags.Include,
ExcludePattern:    pushFlags.Exclude,
```

`cmd/gz-git/cmd/` 아래 `ExcludePattern:`을 채우는 20곳이 모두 같은 모양이다 —
`cleanFlags.Exclude`, `commitFlags.Exclude`, `tagCreateBulkFlags.Exclude` …
설정 파일을 참조하는 곳은 하나도 없다. `push.go`는 설정을 아예 읽지 않는다
(push policy만 `resolvePushPolicy`가 별도로 해석한다).

### 2. `defaults.filter`는 이 스캔에 관여하지 않는다

`defaults.filter.include/exclude`라는 설정 키가 존재하고 문서화도 되어 있어
전역 필터처럼 읽힌다.

```yaml
defaults:
  filter:
    exclude:
      - "archive"
```

그러나 이 값을 읽는 코드는 저장소 전체에 단 한 곳이다.

```go
// pkg/workspacecli/sync_command.go:706 — effectiveForgeWorkspacePatterns
includePatterns = cfg.GetIncludePatterns()
excludePatterns = cfg.GetExcludePatterns()
```

`workspace sync`가 **forge API에서 받아 올 저장소 목록**을 거를 때만 쓰인다
(`docs/usage/workspace-command.md`의 "Forge workspace 필터"). 로컬 디렉터리 스캔과는
무관하므로, 이 키를 설정해도 `push`/`commit`의 대상은 그대로다.

`defaults.`라는 이름이 전역 기본값을 약속하는데 실제 적용 범위는 명령 하나라는 점이
문제의 절반이다. 사용자가 이 키로 막았다고 믿을 근거가 문서에 있다.

### 3. `.gitignore`도 `strategy: skip`도 막지 못한다

- `.gitignore`는 git의 index에 대한 규칙이다. gz-git은 index가 아니라 **파일시스템**을
  훑어 `.git` 디렉터리를 찾으므로 영향을 받지 않는다.
- `strategy: skip`은 sync 계획의 동작 선택이다. 스캔 대상 집합을 줄이지 않는다.
- 설정에 저장소를 **선언하지 않는 것** 역시 효과가 없다. 스캔은 선언 목록이 아니라
  디렉터리를 근거로 하기 때문이다.

이 세 가지는 모두 "막았다고 착각하기 쉬운" 수단이라 위험도가 더 높다.

### 4. 현장 우회책

flow-taskchain-devbox는 이 사실을 `.gz-git.yaml` 주석으로 남기고 디렉터리 자체를
gitignore한 뒤 결국 삭제하는 방식으로 우회했다.

```yaml
# tasuku-repo 는 선언하지 않는다. push 는 선언형이 아니라 디렉터리 스캔 엔진이라
# strategy: skip 으로도 막히지 않는다.
```

우회책이 문서가 아니라 주석으로만 남는다는 것은 도구가 답을 제공하지 않았다는 뜻이다.

## 제안

세 갈래가 있고, 서로 배타적이지 않다.

### A. 로컬 스캔에 적용되는 설정 키 (권장)

`defaults.filter`와 이름이 겹치지 않는 키로, bulk 명령의 스캔 결과를 거른다.

```yaml
scan:
  exclude:
    - "tasuku-repo"
```

플래그가 설정을 덮어쓰되, 설정 제외는 플래그 include로도 되살아나지 않는 편이
안전하다 — 제외의 목적이 "실수 방지"이기 때문이다.

### B. 저장소 단위 read-only 선언

```yaml
repositories:
  - name: tasuku-repo
    readOnly: true    # fetch/pull은 허용, write 명령은 거부
```

A보다 의도가 명확하고 읽기/쓰기를 구분할 수 있지만, 선언 목록과 스캔 결과를 잇는
매칭 규칙이 필요하다(경로 기준인지 remote 기준인지). 구현 비용이 A보다 크다.

### C. `defaults.filter`의 범위를 문서에 명시

A/B와 별개로 즉시 해야 한다. 현재 문서는 이 키가 forge 목록 필터라는 사실을
"Forge workspace 필터" 절 제목으로만 암시한다. 다른 명령에는 효과가 없다는 문장이
있어야 오해가 끊긴다.

## 수용 기준

- [x] `push`가 설정으로 선언된 제외 대상을 스캔 결과에서 뺀다 (A)
- [x] 같은 규칙이 최소한 `commit`, `clean`, `cleanup branch`에도 적용된다 —
  실제로는 `ExcludePattern`을 채우던 24개 호출부 전부에 적용
- [~] 제외된 저장소가 출력에 "제외됨"으로 보고된다 (조용한 성공 금지) —
  **패턴 단위로** 보고한다. 아래 "구현 노트" 참고
- [x] `defaults.filter`의 적용 범위가 `docs/usage/workspace-command.md`에 명시된다 (C)
- [x] 설정 제외와 `--include` 플래그가 충돌할 때의 우선순위가 테스트로 고정된다 —
  `TestConfigExcludeBeatsIncludeFlag`

## 구현 노트 (2026-08-25)

### 키 이름: `scan.exclude`가 아니라 `defaults.scan.exclude`

제안서는 최상위 `scan:`을 썼지만 `defaults.scan.depth`가 이미 존재하는
로컬 스캔 네임스페이스라 그 아래에 뒀다. 최상위에 새 네임스페이스를 만들면
"스캔 설정이 두 군데"라는 문제를 하나 더 만든다.

### 적용 지점: `cmd/` — pkg/repository가 아님

`pkg/config`가 이미 `pkg/repository`를 import한다(`pkg/config/types.go:21`).
따라서 스캐너가 설정을 읽으면 import cycle이다. CLI 계층이 둘을 잇는
유일한 지점이라 `cmd/gz-git/cmd/scan_exclude.go`에 뒀다.

### 우선순위는 코드가 아니라 기존 순서에서 나온다

`filterRepositories`(`pkg/repository/bulk.go:1085-1095`)가 include보다
exclude를 **먼저** 평가한다. 설정 제외를 exclude 정규식에 합치기만 하면
"`--include`로 되살아나지 않는다"는 요구가 추가 코드 없이 충족된다.

### 보고는 패턴 단위 (수용 기준 3의 부분 충족)

"어떤 저장소가 제외됐는지"를 이름으로 나열하려면 스캔 결과를 필터 전후로
비교해야 하고, 그러려면 `ScanOptions`/`BulkUpdateOptions` 등 옵션 구조체
전부에 필드를 하나씩 추가해 `bulkOperationCommon`까지 배선해야 한다.
그 비용에 비해 얻는 것이 "이름 나열"뿐이라 하지 않았다.

대신 적용 중인 패턴을 **매 실행마다** stderr에 출력한다:

```console
$ gz-git push
Excluding repositories matching defaults.scan.exclude: mirror-repo
```

`--dry-run`뿐 아니라 실제 실행에서도 나온다 — push에서 조용히 빠진 저장소는
preview에서 빠진 것만큼 놀랍기 때문이다. stdout이 아니라 stderr인 이유는
`--format json/llm` 출력을 파싱하는 도구를 깨뜨리지 않기 위해서다.
`-q`로 억제된다.

### 잘못된 regex는 fail-closed

`defaults.scan.exclude`의 패턴이 컴파일되지 않으면 경고만 출력하고 넘어가지
않는다. 패턴을 그대로 결합 정규식에 남겨 `filterRepositories`가 스캔을
거부하게 한다. 제외가 조용히 풀린 채 bulk write가 도는 것이 이 이슈가
막으려던 실패 그 자체다.

### 리뷰에서 잡힌 결함 — 빈 항목이 스캔 전체를 비웠다

독립 리뷰가 실제 함수를 실행해 재현했다. `combineExcludePatterns`가 각 패턴을
`(?:...)`로 감싸는데, `(?:)`는 빈 문자열에 매칭되는 유효한 regex다. 그래서
`exclude: [vendor, ""]`는 `(?:vendor)|(?:)`가 되어 **모든 저장소를 제외**했다.
빈 문자열은 문법적으로 유효하므로 위의 fail-closed 컴파일 검사가 발동하지
않았고, stderr 통지 한 줄 외에는 아무 신호가 없었다.

같은 입력이 목록 길이에 따라 정반대로 동작한 것이 더 나쁘다. `exclude: [""]`
하나뿐이면 `len==1` 단축 경로가 `""`를 그대로 돌려주고,
`filterRepositories`는 그것을 "제외 필터 없음"으로 읽어 제외가 사라졌다.

`dropEmptyPatterns`가 빈 항목을 버리고 몇 번째 항목이었는지 stderr에 알린다.
치명적 오류로 만들지 않은 이유는 빈 항목이 잃을 제외를 애초에 담고 있지
않기 때문이다 — 실제로 쓰인 패턴 그대로 실행된다. `combineExcludePatterns`
자체에도 같은 방어를 넣어 호출자가 무엇을 넘기든 전체 매칭 정규식을 만들 수
없게 했다. 공백만 있는 항목(`" "`)은 공백에 매칭되는 진짜 regex이므로 건드리지
않는다.

### 파생 발견 — `defaults.scan.depth`도 죽은 키다

`GetScanDepth()`(`pkg/config/types.go:983`)의 소비처가 저장소 전체에 없다.
`defaults.filter`와 정확히 같은 부류의 결함이라 별도 카드로 분리했다:
`tasks/issue/26-defaults-scan-depth-is-a-dead-config-key.md`

## 참고

- `cmd/gz-git/cmd/push.go:144-146` — 스캔 옵션 조립
- `pkg/config/types.go:196-199` — `FilterDefaults`
- `pkg/config/types.go:1006-1020` — `GetIncludePatterns` / `GetExcludePatterns`
- `pkg/workspacecli/sync_command.go:703-716` — 유일한 소비처
- `docs/usage/workspace-command.md:351-375` — Forge workspace 필터
