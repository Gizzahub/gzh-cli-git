# ISSUE: `make changelog`가 수기 `CHANGELOG.md`를 통째로 덮어쓴다

- status: open
- priority: P2
- category: build / tooling
- created_at: 2026-08-24T23:40:00+09:00
- affects: `CHANGELOG.md` 63KB 수기 서술 — 실행 즉시 커밋 제목 나열로 대체된다
- spawned_from: 이슈 22의 분할 설계를 검토하다 릴리스 자동화 경로를 확인

## 요약

`.make/dev.mk`에 생성 타깃이 있다:

```make
changelog: ## generate changelog (requires git-chglog)
	@command -v git-chglog >/dev/null 2>&1 || { echo "git-chglog not found..."; exit 1; }
	@git-chglog -o CHANGELOG.md
```

`-o CHANGELOG.md`는 **추가가 아니라 덮어쓰기**다. 그런데 이 저장소의 `CHANGELOG.md`는
생성물이 아니라 **손으로 쓴 서술 문서**다. 44개 커밋에 걸쳐 기능 커밋마다 사람이 문단을
직접 붙여 왔고, 각 항목은 "무엇을 바꿨나"가 아니라 "왜 그렇게 바꿨나"를 여러 문단으로
설명한다. git-chglog가 만드는 것은 커밋 제목 목록이다. 실행되면 그 서술이 전부 사라진다.

## 지금 터지지 않는 이유 — 그리고 그게 왜 안심할 근거가 아닌지

두 가지 우연이 막고 있을 뿐이다.

| 조건 | 현재 상태 |
| ----------------------- | -------------------------------- |
| `git-chglog` 설치 | **미설치** → 타깃이 `exit 1`로 죽는다 |
| `.chglog/` 설정 디렉터리 | **없음** → 설치해도 config 없이 실패한다 |

즉 지금은 "동작하지 않는 타깃"이다. 문제는 이 둘 다 **누군가 친절을 베풀면 사라지는
조건**이라는 점이다. `make help`에 `changelog`가 노출돼 있으니, 릴리스를 준비하던 사람이
"도구가 없다길래 설치했다"를 두 번 하면 63KB가 날아간다. 죽은 코드가 아니라 **장전된
총**이다.

되돌릴 수는 있다(git이 있다). 하지만 되돌려야 한다는 사실을 알아채는 것은 별개 문제다 —
덮어쓴 결과물도 그럴듯한 CHANGELOG처럼 보인다.

## 이슈 22와의 충돌

[22-changelog-exceeds-doc-size-gate](22-changelog-exceeds-doc-size-gate.md)는 과거 릴리스를
`docs/changelog/`로 옮기고 `CHANGELOG.md`에는 인덱스만 남기는 구조를 세운다. `git-chglog -o
CHANGELOG.md`는 그 인덱스를 **전체 이력 재생성으로 대체**한다. 두 설계는 공존할 수 없다.
그래서 이 이슈가 22의 선행 조건이다.

## 후보 해법

| 안 | 내용 | 평가 |
| -- | ------------------------------------------- | ------------------------------------------------- |
| A | 타깃 삭제 | 실제 규약(수기 관리)과 일치. 가장 단순 |
| B | `.chglog/` 설정을 추가하고 생성형으로 전환 | 서술의 가치를 버린다. 이 파일의 강점이 바로 그것 |
| C | 출력 경로를 `CHANGELOG.generated.md`로 변경 | 파괴는 막지만 아무도 안 쓰는 산출물이 하나 더 는다 |

**권장은 A.** 근거: (1) 설정도 도구도 없이 방치돼 실제로 쓰인 적이 없고, (2) 실행되면
파괴적이며, (3) 기계적 릴리스 목록은 이미 `.goreleaser.yaml`의 `changelog.use: github`가
GitHub Release 노트로 생성하고 있어 **기능이 중복**이다. 생성형이 정말 필요해지면 그때 B를
설정과 함께 정식 도입하는 편이, 지금 반쯤 살려 두는 것보다 안전하다.

## Acceptance Criteria

- [ ] `make changelog`가 `CHANGELOG.md`를 덮어쓸 수 없다 (타깃 제거 또는 출력 경로 변경)
- [ ] `make help` 출력에 오해를 부르는 항목이 남지 않는다
- [ ] 릴리스 절차 문서가 있다면 `make changelog`를 참조하지 않는다
- [ ] 기계적 릴리스 목록의 출처가 `.goreleaser.yaml`임이 문서에 남는다

## 범위 경계

- `.goreleaser.yaml`의 changelog 설정은 건드리지 않는다. 그쪽은 정상 동작하며 역할이 다르다.
- `CHANGELOG.md`의 내용·구조 변경은 이슈 22 소관이다.

## References

- `.make/dev.mk` — `changelog` 타깃
- `.goreleaser.yaml` — `changelog.use: github`, `release.mode: replace`
- 선행/후속: [22-changelog-exceeds-doc-size-gate](22-changelog-exceeds-doc-size-gate.md)
