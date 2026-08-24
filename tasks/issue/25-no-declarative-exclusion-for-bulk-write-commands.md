# ISSUE: bulk write 명령을 저장소 단위로 막을 선언형 수단이 없다

- status: open
- priority: P2
- category: config / safety
- created_at: 2026-08-25T00:20:00+09:00
- affects: `push`, `commit`, `clean`, `cleanup branch`, `stash`, `tag create`, `pr create`, `switch`, `update`, `exec` — 매 실행마다 `--exclude`를 기억해야만 특정 저장소를 건드리지 않는다
- spawned_from: flow-taskchain-devbox의 `tasuku-repo` 제외 처리를 검토하다 발견

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

- [ ] `push`가 설정으로 선언된 제외 대상을 스캔 결과에서 뺀다 (A 또는 B)
- [ ] 같은 규칙이 최소한 `commit`, `clean`, `cleanup branch`에도 적용된다
- [ ] 제외된 저장소가 `--dry-run` 출력에 "제외됨"으로 보고된다 (조용한 성공 금지)
- [ ] `defaults.filter`의 적용 범위가 `docs/usage/workspace-command.md`에 명시된다 (C)
- [ ] 설정 제외와 `--include` 플래그가 충돌할 때의 우선순위가 테스트로 고정된다

## 참고

- `cmd/gz-git/cmd/push.go:144-146` — 스캔 옵션 조립
- `pkg/config/types.go:196-199` — `FilterDefaults`
- `pkg/config/types.go:1006-1020` — `GetIncludePatterns` / `GetExcludePatterns`
- `pkg/workspacecli/sync_command.go:703-716` — 유일한 소비처
- `docs/usage/workspace-command.md:351-375` — Forge workspace 필터
