# 28. `.goreleaser.yaml`의 `brews`가 폐기 예정인데 CI는 `version: latest`를 쓴다

> 상태: 완료 (2026-08-25) — 저장소 소유 코드·CI 범위 해결
> 발견: 2026-08-25, 릴리스 드라이런 중
> 관련: 이 저장소에는 아직 git 태그가 없어 실제 릴리스는 한 번도 돈 적이 없다

## 해결

권장했던 버전 고정과 Cask 이전을 함께 완료했다.

- `24944dc`: snapshot/release 워크플로의 GoReleaser를 로컬 도구와 같은
  `v2.10.2`로 고정했다.
- `5be5005`: `brews`를 `homebrew_casks`로 이전하고 설치 문서와 릴리스 안내를
  Cask 기준으로 맞췄다.
- `5cbd428`: 저장소의 이동 `snapshot` 태그를 버전 탐색에서 제외하고, 생성된
  `dist/homebrew/Casks/gz-git.rb`를 `ruby -c`로 검사한 뒤에만 artifact를 게시하도록
  게이트를 보강했다.

검증은 `goreleaser check`, 실제 `goreleaser release --snapshot --clean`, 생성된 Cask의
Ruby 구문 검사, `GOWORK=off make quality-check`, 통합 시 `make check`·`make lint`로
완료했다. 새 clone과 깨끗한 모듈 캐시에서도 snapshot 5개 artifact 생성을 확인했다.

외부 `gizzahub/homebrew-tap`의 기존 Formula 사용자가 Cask로 자연스럽게 업그레이드되는지는
이 저장소의 코드 결함과 분리한 **릴리스 운영 선행조건**이다. 실제 tap fixture에서 전환을
검증한 뒤 기존 Formula를 폐기하거나 제거해야 하며, 같은 이름을 자기 자신으로 매핑하는
`tap_migrations.json`이 필요하다고 가정하지 않는다.

## 증상

`goreleaser check`가 실패한다. 설정 자체는 유효하지만 폐기된 키를 쓴다.

```
$ goreleaser check
  • checking                                         path=.goreleaser.yaml
  • DEPRECATED:  brews  should not be used anymore, check https://goreleaser.com/deprecations#brews for more info
  • .goreleaser.yaml                                 error=configuration is valid, but uses deprecated properties
  ⨯ command failed                                   error=1 out of 1 configuration file(s) have issues
```

빌드는 아직 성공한다 — `goreleaser build --snapshot`은 같은 경고를 내고 정상 종료한다.
즉 **오늘 릴리스를 끊는 것 자체는 막히지 않는다.** 문제는 그 다음이다.

## 왜 지금 문제인가 — 시한폭탄의 실제 구조

두 사실이 겹쳐서 위험해진다.

1. `.goreleaser.yaml:131` `brews:` — GoReleaser가 Homebrew Formula 대신 Cask로
   이전하면서 폐기 예고한 키다. 폐기 예고는 언젠가 제거로 끝난다.
1. `.github/workflows/release.yml` — `goreleaser-action@v7`을 `version: latest`로
   고정 없이 쓴다.

`latest`는 릴리스 태그를 미는 그 시점에 최신인 GoReleaser를 가져온다는 뜻이다.
`brews`가 제거된 버전이 나오는 순간, **저장소를 아무것도 건드리지 않았는데도**
다음 태그 푸시에서 릴리스 워크플로가 깨진다. 깨지는 시점을 이 저장소가 통제하지
못하고, 하필 릴리스를 끊는 순간에만 드러난다.

## 이전 방법과 그 대가

GoReleaser 문서가 제시하는 이전은 기계적이다.

```yaml
homebrew_casks:
  - name: gz-git
    ids: [default]
    repository:
      owner: gizzahub
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    homepage: "https://github.com/gizzahub/gzh-cli-gitforge"
    description: "Git repository management CLI tool with multi-forge support"
    license: "MIT"
    # 서명·공증하지 않은 바이너리는 macOS 격리 속성을 직접 떼야 한다
    hooks:
      post:
        install: |
          if OS.mac?
            system_command "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "#{staged_path}/gz-git"]
          end
```

하지만 설정 파일만 바꿔서 끝나지 않는다. 대가가 셋이다.

### 1. 외부 저장소 `gizzahub/homebrew-tap`의 업그레이드 경로 검증이 필요하다

이미 `brew install gizzahub/tap/gz-git`으로 설치한 사용자는 Formula를 들고 있다.
기존 Formula와 새 Cask가 같은 token(`gz-git`)을 쓰는 전환은 실제 tap fixture에서
`brew upgrade` 동작을 확인해야 한다. 조사 당시 `tap_migrations.json`이 반드시 필요하다고
판단했지만, 같은 이름을 자기 자신으로 매핑하는 migration은 근거가 없어 채택하지 않았다.
검증이 끝날 때까지 기존 Formula 폐기·제거는 보류한다. 외부 tap 변경은 이 저장소의
코드 수정과 별도 운영 작업이다.

### 2. Linux 설치 경로가 사라질 수 있다

현재 릴리스 노트 footer(`.goreleaser.yaml:117`)는 설치 방법을
**"Homebrew (macOS/Linux)"** 로 광고한다. Formula는 Linuxbrew에서도 설치되지만
Cask는 Homebrew에서 macOS 전용으로 취급된다. 이전하면 광고한 Linux 경로가
없어질 가능성이 높다 — **이 부분은 실제 Linuxbrew 환경에서 확인한 사실이 아니라
문서상 추정이므로, 결정 전에 검증이 필요하다.** 참이라면 footer 문구도 함께
고쳐야 하고, Linux 사용자에게는 `go install`과 바이너리 다운로드만 남는다.

### 3. `binary` / `binaries` 필드가 GoReleaser 버전에 따라 다르다

`homebrew_casks.binary`는 v2.12.6에서 복수형 `binaries`로 이름이 바뀌었다.
이 워크스테이션에 설치된 GoReleaser는 v2.10.2다(`.make/tools.mk:68`).
즉 **로컬에서 통과하는 설정이 CI(`latest`)에서는 폐기 경고를 내거나 그 반대일 수
있다.** 이전 작업을 하려면 로컬과 CI의 GoReleaser 버전을 먼저 일치시켜야 한다.

## 제안

두 갈래를 분리해서 처리한다. 두 번째가 첫 번째보다 급하다.

**(a) `brews` → `homebrew_casks` 이전** — 완료.
저장소 설정과 문서는 Cask 기준으로 바꿨다. 외부 tap의 기존 Formula 전환 검증은
릴리스 운영 선행조건으로 분리했다.

**(b) CI의 GoReleaser 버전 고정** — 완료.
`.github/workflows/release.yml`의 `version: latest`를 `.make/tools.mk`가 설치하는 버전과
같은 명시적 버전으로 바꿨다. 그러면 릴리스가 깨지는 시점이
"어느 날 갑자기"에서 "우리가 버전을 올릴 때"로 옮겨오고, 로컬 드라이런 결과가
CI 결과를 실제로 예측하게 된다.

## 채택하지 않은 대안

- **`--skip=publish`나 경고 무시로 넘기기** — `goreleaser check`를 게이트에서
  빼는 것과 같다. 폐기 경고는 그 자체로 유효한 신호인데 신호를 끄는 셈이고,
  제거되는 날 그대로 터진다.
- **`brews` 블록 삭제** — Homebrew 배포를 그냥 포기하는 것. footer가 광고하는
  설치 경로를 아무 대체 없이 없애므로 (a)의 결정을 우회하는 게 아니라 더 나쁜
  쪽으로 미리 답해버린다.
- **tap 저장소를 이 저장소가 자동으로 고치기** — 다른 저장소의 사용자 대면
  설치 경로를 릴리스 파이프라인이 조용히 바꾸는 것. 실패해도 이쪽에서 안 보인다.
