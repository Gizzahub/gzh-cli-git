# gz-git doctor

시스템 의존성, 설정 무결성, 인증, 포지 연결, 저장소 상태를 진단하는 명령어.

## 기본 사용법

```bash
# 전체 진단
gz-git doctor

# 저장소 2단계 깊이 스캔
gz-git doctor -d 2

# 상세 출력 (프로필별, 브랜치별 정보 포함)
gz-git doctor -v

# 포지/저장소 체크 생략
gz-git doctor --skip-forge
gz-git doctor --skip-repo

# JSON 출력 (스크립트/CI 연동)
gz-git doctor --format json
```

## 출력 예시

```
gz-git doctor -d 1

System
  ✓ git version 2.53.0
  ✓ ssh available
  ✓ temp directory writable (/tmp)
  ✓ config directory: ~/.config/gz-git
Configuration
  ✓ global config: ~/.config/gz-git/config.yaml
  ✓ active profile: default
  ✓ 2 profile(s) found, all valid
  ✓ project config: ~/mydevbox/.gz-git.yaml
Authentication
  ✓ default SSH key found: ~/.ssh/id_ed25519
Forge Connectivity
  ✓ profile 'work': gitlab token valid (rate limit: 1800/2000, 90%)
  ⚠ profile 'oss': github rate limit low (8% remaining)
Repositories
  ✗ my-app: no remote configured
  ✗ api-server: merge in progress
  ✗ web-client: dirty worktree + 5 commits behind upstream
  ⚠ shared-lib: diverged from upstream (3 ahead, 12 behind)
  ⚠ core: develop is 85 commits from main

Checks: 16 total, 10 ok, 3 warning, 3 error (1.5s)
```

## 상태 아이콘

| 아이콘 | 상태 | 의미 |
|--------|------|------|
| `✓` | ok | 정상 |
| `⚠` | warning | 비치명적 이슈 (동작에 영향 없음) |
| `✗` | error | 치명적 이슈 (수정 필요) |
| `⊘` | unreachable | 대상에 연결 불가 |

## 진단 카테고리

### System

| 체크 항목 | 설명 |
|-----------|------|
| git | git 설치 여부 및 버전 |
| ssh | ssh 명령어 가용성 |
| temp-dir | 임시 디렉토리 쓰기 권한 |
| config-dir | `~/.config/gz-git/` 존재 및 권한 (0700) |

### Configuration

| 체크 항목 | 설명 |
|-----------|------|
| global-config | 전역 설정 파일 파싱 가능 여부 |
| active-profile | 활성 프로필 존재 여부 |
| profile:{name} | 각 프로필 유효성 검증 (`-v` 시 개별 표시) |
| project-config | 현재 디렉토리의 `.gz-git.yaml` 탐색 |

### Authentication

| 체크 항목 | 설명 |
|-----------|------|
| ssh-key:{profile} | 프로필에 설정된 SSH 키 존재 및 권한 (0600) |
| ssh-key:default | 기본 SSH 키 탐지 (`id_ed25519`, `id_rsa`, `id_ecdsa`) |

### Forge Connectivity

| 체크 항목 | 설명 |
|-----------|------|
| forge:{profile} | API 토큰 유효성, rate limit 잔여량 |

### Repositories

| 체크 항목 | 설명 | 임계값 |
|-----------|------|--------|
| repo:{name}:remote | remote 미설정 (sync 불가) | - |
| repo:{name}:detached | HEAD가 detached 상태 | - |
| repo:{name}:merge | merge 진행 중 (미완료) | - |
| repo:{name}:rebase | rebase 진행 중 (미완료) | - |
| repo:{name}:conflict | 머지 충돌 파일 존재 | - |
| repo:{name}:dirty-behind | dirty worktree + upstream behind | - |
| repo:{name}:diverged | local/upstream 분기 (ahead+behind 동시) | behind > 10: error, 그 외 warning |
| repo:{name}:behind | upstream 대비 뒤처짐 (behind만) | > 10 commits: warning |
| repo:{name}:ahead | 미push 커밋 (ahead만) | > 20 commits: warning |
| repo:{name}:develop-main | develop ↔ main/master 거리 | warn: 50, error: 150 |
| repo:{name}:branch:{branch} | feature 브랜치 분기 거리 (`-v` 시) | warn: 30, error: 100 |
| repo:{name}:stash | 다른 장비에서 볼 수 없는 오래된 stash | 가장 오래된 항목이 7일 초과: warning |

## 플래그

| 플래그 | 설명 | 기본값 |
|--------|------|--------|
| `-d, --scan-depth` | 저장소 스캔 깊이 | `1` |
| `--skip-forge` | 포지 API 연결 체크 생략 | `false` |
| `--skip-repo` | 저장소 상태 체크 생략 | `false` |
| `--format` | 출력 형식 (`json`) | 기본 텍스트 |
| `-v, --verbose` | 프로필별/브랜치별 상세 출력 | `false` |

## JSON 출력

```bash
gz-git doctor --format json
```

```json
{
  "checks": [
    {
      "name": "git",
      "category": "system",
      "status": "ok",
      "message": "git version 2.53.0"
    }
  ],
  "summary": {
    "ok": 9,
    "warning": 1,
    "error": 0,
    "unreachable": 0,
    "skipped": 0,
    "total": 10
  },
  "duration": 1200000
}
```

## 일반적인 문제와 해결

| 증상 | 해결 |
|------|------|
| `✗ git is not installed` | git 설치 후 PATH에 추가 |
| `⚠ config directory does not exist` | `gz-git config init` 실행 |
| `✗ active profile 'work' does not exist` | `gz-git config profile create work` |
| `⚠ profile 'work': env var $TOKEN is empty` | 환경변수 설정 (`export TOKEN=...`) |
| `✗ SSH key has loose permissions: 0644` | `chmod 600 ~/.ssh/id_ed25519` |
| `⊘ gitlab API unreachable` | 네트워크 연결 또는 base URL 확인 |
| `⚠ rate limit low` | 잠시 대기 후 재시도, 또는 토큰 교체 |
| `✗ no remote configured` | `git remote add origin <url>` |
| `✗ merge in progress` | `git merge --continue` 또는 `--abort` |
| `✗ rebase in progress` | `git rebase --continue` 또는 `--abort` |
| `✗ N file(s) with merge conflicts` | 충돌 파일 수정 후 `git add` |
| `✗ dirty worktree + N behind` | 변경사항 commit/stash 후 pull |
| `⚠ N stash entries, oldest N days old` | `git stash pop` 후 커밋 — stash는 다른 장비로 전달되지 않음 |
| `⚠ diverged from upstream` | `git pull --rebase` 또는 `git merge` |
| `⚠ develop is N commits from main` | develop → main 머지 고려 |
| `⚠ branch 'feat/x' is N commits from base` | base 브랜치에 rebase |
