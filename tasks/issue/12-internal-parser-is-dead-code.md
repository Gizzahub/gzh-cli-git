# ISSUE: `internal/parser` 패키지 전체가 죽은 코드다

- status: todo
- priority: P3
- category: cleanup
- created_at: 2026-08-06T10:00:00+09:00
- affects: v0.7.0+ (기능 영향 없음 — 유지보수 부채)
- spawned_from: `10-porcelain-parsers-outside-pkg-repository.md` §5

## Background

태스크 06이 `internal/parser.ParseStatus`를 삭제했다. 삭제 근거는 "프로덕션 호출자
0건"이었다. 그때 확인하지 않은 것은 **나머지 함수도 전부 같은 상태라는 사실**이다.

## Findings

```
$ grep -rn --include='*.go' 'gzh-cli-gitforge/internal/parser' . | grep -v '^\./internal/parser/'
(no matches, exit 1)
```

패키지를 임포트하는 파일이 **모듈 전체에 0개**다. 남은 것은 자기 자신을 테스트하는
테스트뿐이다.

| 파일 | 크기 | 내용 |
|---|---|---|
| `internal/parser/status.go` | 2.8KB | 7 exported (`ParseBranchInfo`, `ParseRemoteInfo`, `ParseUpstreamInfo`, `ParseAheadBehind`, `ParseCommitInfo`, `ParseFileList`, `ParseIsClean`) |
| `internal/parser/common.go` | 6.1KB | 13 exported (`SplitLines`, `ParseKeyValue`, `ParseInt`, `ParseBool`, `ParseTimestamp`, `ParseDate`, `ParseRef`, `ParseCommitHash`, `ParseFileMode`, `IsEmptyLine`, `TrimPrefix`, `SplitFields`, `ExtractBetween`) |
| `internal/parser/status_test.go` | 7.6KB | 위 7개 테스트 |
| `internal/parser/common_test.go` | 13.5KB | 위 13개 테스트 |

**21KB의 테스트가 아무도 호출하지 않는 8.9KB의 코드를 검증하고 있다.**

### 왜 단순 정리 이상인가

`pkg/doctor`는 `parseAheadBehind` **사본을 따로 갖고 있다**. 즉 이름이 같은 두 구현이
공존하며, 실제로 쓰이는 쪽은 사본이다. 지금 상태에서 누군가 "ahead/behind 파싱 버그"를
고치러 들어오면 `internal/parser.ParseAheadBehind`를 먼저 찾을 가능성이 높고,
그 수정은 **어떤 실행 경로에도 도달하지 않는다.** 커버리지 통계도 같은 방식으로
왜곡된다 — 죽은 코드가 100% 커버리지로 잡힌다.

`internal/`은 컴파일러가 강제하는 가시성 경계이므로 **외부 호환성 의무가 없다.**
deprecate 기간 없이 바로 지울 수 있다.

## Scope

1. 삭제 직전에 임포터 0건을 다시 확인한다 (`grep -rn --include='*.go'
   'gzh-cli-gitforge/internal/parser' .`). 태스크 작성 시점 이후 새 호출자가 생겼다면
   중단하고 그 호출자를 먼저 판단한다.
2. `internal/parser/` 디렉터리 전체 삭제 (소스 2 + 테스트 2).
3. `pkg/doctor`의 `parseAheadBehind` 사본은 **그대로 둔다.** 이 태스크는 죽은 코드
   제거이지 파서 통합이 아니다 — 살아있는 유일한 구현을 건드릴 이유가 없다.
   (모듈 전역 porcelain 파서는 이미 `internal/porcelain`이 담당한다.)
4. `docs/`·`CLAUDE.md`에 `internal/parser` 언급이 있으면 정정한다.
   루트 `CLAUDE.md`의 디렉터리 구조표에 `parser/ # Output parsing` 항목이 있다.

## Acceptance Criteria

- [ ] `internal/parser/` 부재
- [ ] `go build ./...` · `go vet ./...` 통과
- [ ] `make quality` 통과
- [ ] `grep -rn 'internal/parser' .` 결과가 문서/태스크 이력에만 남음
- [ ] 루트 `CLAUDE.md` 디렉터리 구조표에서 `parser/` 항목 제거
- [ ] CHANGELOG 기재 (`internal/`이라 공개 API 변경 아님을 명시)

## References

- 발견 경위: `10-porcelain-parsers-outside-pkg-repository.md` (§5, Follow-ups)
- 선례: 태스크 06의 `internal/parser.ParseStatus` 삭제 — 같은 근거, 같은 결론
- 현행 파서: `internal/porcelain` (모듈 전역), `pkg/repository/porcelain.go` (패키지 내부)
