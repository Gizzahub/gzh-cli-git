# 28. `.goreleaser.yaml`의 `brews`가 폐기 예정인데 CI는 `version: latest`를 쓴다

> 상태: 진행 중 — 저장소 코드·CI 완료, 최초 stable release/tap bootstrap 검증 대기
> 발견: 2026-08-25, 릴리스 드라이런 중
> 관련: 이동 `snapshot` 태그는 있지만 stable `v*` 태그와 실제 릴리스 이력은 없다

## 구현 완료 범위

권장했던 버전 고정과 Cask 이전을 함께 완료했다.

- `24944dc`: snapshot/release 워크플로의 GoReleaser를 로컬 도구와 같은
  `v2.10.2`로 고정했다.
- `5be5005`: `brews`를 `homebrew_casks`로 이전하고 설치 문서와 릴리스 안내를
  Cask 기준으로 맞췄다.
- `5cbd428`: 저장소의 이동 `snapshot` 태그를 버전 탐색에서 제외하고, 생성된
  `dist/homebrew/Casks/gz-git.rb`를 `ruby -c`로 검사한 뒤에만 artifact를 게시하도록
  게이트를 보강했다.

검증은 `goreleaser check`, 로컬 `goreleaser release --snapshot --clean`, 생성된 Cask의
Ruby 구문 검사, `GOWORK=off make quality-check`, 통합 시 `make check`·`make lint`로
수행했다. 새 clone과 깨끗한 모듈 캐시에서도 로컬 snapshot artifact 5개 생성을 확인했다.

## 남은 수용 기준

2026-08-25 기준 외부 `gizzahub/homebrew-tap`은 ref가 없는 빈 저장소다. 기존 Formula나
Formula 사용자를 전제한 migration은 필요하지 않다. 최초 stable release 전에 다음
**릴리스 운영 선행조건**을 실제 외부 경로에서 검증한다.

1. tap의 기본 브랜치와 README를 초기화한다.
1. `HOMEBREW_TAP_TOKEN`으로 `Casks/gz-git.rb` 최초 게시가 되는지 확인한다.
1. 생성된 Cask를 audit하고 macOS에서 tap을 통한 설치와 버전 출력을 확인한다.
1. stable `v*` 태그의 release workflow가 완료된 실행 근거를 남긴다.

이 검증 전에는 이슈를 완료 처리하지 않는다. Linux 사용자는 Cask 지원을 약속하지 않고
`go install` 또는 바이너리 다운로드 경로를 사용한다.

## 발견 당시 증상

수정 전 `goreleaser check`가 실패했다. 설정 자체는 유효하지만 폐기된 키를 썼다.

```
$ goreleaser check
  • checking                                         path=.goreleaser.yaml
  • DEPRECATED:  brews  should not be used anymore, check https://goreleaser.com/deprecations#brews for more info
  • .goreleaser.yaml                                 error=configuration is valid, but uses deprecated properties
  ⨯ command failed                                   error=1 out of 1 configuration file(s) have issues
```

수정 전 빌드는 성공했다. `goreleaser build --snapshot`은 같은 경고를 내고 정상
종료했으므로 당장의 build보다 이후 버전에서 제거될 설정이 문제였다.

## 발견 당시 위험 구조

두 사실이 겹쳐서 위험했다.

1. 당시 `.goreleaser.yaml`의 `brews:`는 GoReleaser가 Homebrew Formula 대신 Cask로
   이전하면서 폐기 예고한 키였다.
1. 당시 snapshot/release workflow는 `goreleaser-action@v7`을 `version: latest`로
   고정 없이 썼다.

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

### 1. 외부 tap bootstrap 검증이 필요하다

조사 중 기존 Formula를 전제했지만 실제 원격 `gizzahub/homebrew-tap`은 ref가 없는 빈
저장소였다. 따라서 Formula migration이 아니라 기본 브랜치 초기화, 최초 Cask 게시,
macOS 설치 검증이 필요하다. 외부 tap bootstrap은 이 저장소의 코드 수정과 별도
릴리스 운영 작업이다.

### 2. Linux를 Homebrew Cask 지원 대상으로 광고하지 않는다

수정 전 릴리스 footer는 **"Homebrew (macOS/Linux)"** 를 광고했다. Cask 전환 후에는
검증하지 않은 Linux 지원 약속을 제거하고 Homebrew Cask를 macOS 설치 경로로만
표시했다. Linux 사용자는 `go install`과 바이너리 다운로드 경로를 사용한다.

### 3. `binary` / `binaries` 필드가 GoReleaser 버전에 따라 다르다

`homebrew_casks.binary`는 v2.12.6에서 복수형 `binaries`로 이름이 바뀌었다.
이 워크스테이션에 설치된 GoReleaser는 v2.10.2다(`.make/tools.mk:68`).
수정 전에는 **로컬에서 통과하는 설정이 CI(`latest`)에서는 폐기 경고를 내거나 그
반대일 수 있었다.** 로컬과 두 CI workflow를 `v2.10.2`로 일치시켜 해소했다.

## 당시 제안과 채택 결과

두 갈래를 분리했고 버전 고정을 먼저 적용한 뒤 Cask 이전을 진행했다.

**(a) `brews` → `homebrew_casks` 이전** — 완료.
저장소 설정과 문서는 Cask 기준으로 바꿨다. 빈 외부 tap의 bootstrap과 최초 게시·설치
검증은 이 이슈의 남은 릴리스 운영 수용 기준으로 분리했다.

**(b) CI의 GoReleaser 버전 고정** — 완료.
`.github/workflows/release.yml`의 `version: latest`를 `.make/tools.mk`가 설치하는 버전과
같은 명시적 버전으로 바꿨다. 그 결과 릴리스가 깨지는 시점이
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
