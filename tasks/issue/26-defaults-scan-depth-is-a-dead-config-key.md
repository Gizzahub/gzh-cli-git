# ISSUE: `defaults.scan.depth`는 아무 명령도 읽지 않는 죽은 설정 키다

- status: closed
- priority: P3
- category: config
- created_at: 2026-08-25T00:00:00+09:00
- affects: `-d/--scan-depth`를 쓰는 모든 bulk 명령 — 설정으로 기본 깊이를 바꿀 수 없다
- spawned_from: issue 25(`defaults.scan.exclude`) 구현 중 같은 네임스페이스를 읽다가 발견

## 요약

`defaults.scan.depth`는 파싱되고, 병합되고, getter까지 있는데 **읽는 곳이 없다**.
설정에 써 두면 조용히 무시되고 항상 플래그 기본값 1이 쓰인다.

`defaults.filter`(issue 25)와 정확히 같은 부류의 결함이다: 설정 키가 존재하고
동작하는 것처럼 보이지만 실제 소비처가 없거나 다른 범위에만 적용된다.

## 해결 (2026-08-25)

권장안대로 키를 유지하고 실제 bulk directory scan의 기본값으로 연결했다.
명시적 `--scan-depth`가 설정을 이기며, 부모 설정에서 상속된 깊이와 프로젝트 설정의
`.yaml`·`.yml`·`.json` 탐색 우선순위도 동일하게 적용된다.

스캔하지 않는 `tag auto`에서는 오해를 부르는 bulk 플래그를 제거했다. Cobra 명령 트리를
반복 실행하는 테스트·임베딩 환경에서도 Args/help/flag parse 오류 뒤의 깊이가 다음 실행에
남지 않도록 실제 `Execute` 경계에서 `scan-depth` 상태를 복원한다.

`depth: 0`은 기존 재귀 병합 계약대로 “미설정”이다. 자식에서는 부모 값을 상속하고,
최상위에서는 CLI 기본값 1을 유지한다. 양수만 사용자 지정 깊이로 적용된다.

구현 커밋: `7fa61c5`, `341ce6d`, `bfc6e07`, `9924656`, `9a2c12f`.

아래 사실관계와 제안은 발견 당시의 기록으로 보존한다.

## 발견 당시 사실관계

### 1. getter의 소비처가 저장소 전체에 없다

```go
// pkg/config/types.go:983
func (c *Config) GetScanDepth() int {
	if c.Defaults != nil && c.Defaults.Scan != nil {
		return c.Defaults.Scan.Depth
	}
	return 0
}
```

```console
$ grep -rn "GetScanDepth" --include="*.go" .
pkg/config/types.go:983:func (c *Config) GetScanDepth() int {
```

정의 한 줄뿐이다. 호출부도, 테스트도 없다.

### 2. 병합 로직은 멀쩡히 살아 있어서 더 헷갈린다

```go
// pkg/config/recursive.go:487-493
if parent.Defaults.Scan != nil {
	if child.Defaults.Scan == nil {
		child.Defaults.Scan = &ScanDefaults{}
	}
	if child.Defaults.Scan.Depth == 0 {
		child.Defaults.Scan.Depth = parent.Defaults.Scan.Depth
	}
```

부모/자식 설정 병합까지 구현돼 있으니 코드를 읽는 사람은 당연히 동작한다고 본다.

### 3. 실제 깊이는 언제나 플래그 기본값이다

```go
// cmd/gz-git/cmd/bulk_common.go:77
cmd.Flags().IntVarP(&flags.Depth, "scan-depth", "d",
	repository.DefaultLocalScanDepth, "directory depth to scan for repositories")
```

`bulk_common.go`가 설정을 참조하지 않으므로 `defaults.scan.depth`가 무엇이든
`--scan-depth`를 직접 주지 않는 한 1이다.

## 제안

`defaults.scan.exclude`(issue 25)가 이미 `cmd/gz-git/cmd/scan_exclude.go`에서
설정을 읽어 오는 경로를 만들어 뒀으므로, 그 자리에 붙이는 것이 자연스럽다.

플래그 기본값과 설정값을 구분하려면 cobra의 `cmd.Flags().Changed("scan-depth")`가
필요하다 — 플래그를 주지 않았을 때만 설정값을 쓰고, 명시했으면 플래그가 이긴다.

대안은 키를 **삭제**하는 것이다. 동작하지 않는 키를 남겨 두는 비용이
없는 기능을 제공하지 않는 비용보다 크다면 그쪽이 정직하다.

## 수용 기준

- [x] `defaults.scan.depth`가 설정된 상태에서 `--scan-depth` 없이 실행하면
  그 깊이가 적용된다 — 또는 키가 제거되고 문서에서도 사라진다
- [x] `--scan-depth`를 명시하면 설정값을 이긴다 (테스트로 고정)
- [x] 부모 설정에서 상속된 깊이도 동일하게 적용된다

## 참고

- `pkg/config/types.go:982-988` — `GetScanDepth`
- `pkg/config/recursive.go:486-494` — 병합
- `cmd/gz-git/cmd/bulk_common.go:76-78` — 플래그 기본값
- `tasks/issue/25-no-declarative-exclusion-for-bulk-write-commands.md` — 같은 결함 부류
- `cmd/gz-git/cmd/scan_exclude.go` — 설정을 읽어 오는 기존 경로
