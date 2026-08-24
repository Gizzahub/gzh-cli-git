# gz-git config

Profile 및 설정 관리.

## 서브커맨드

| 커맨드      | 설명                                       |
| ----------- | ------------------------------------------ |
| `init`      | Config 디렉토리 초기화                     |
| `profile`   | Profile 관리 (create/use/list/show/delete) |
| `show`      | 현재 설정 표시                             |
| `hierarchy` | Config 계층 트리 표시                      |

## init

Config 디렉토리 초기화.

```bash
gz-git config init
```

생성되는 구조:

```
~/.config/gz-git/
├── config.yaml           # Global config
├── profiles/
│   └── default.yaml      # Default profile
└── state/
    └── active-profile.txt
```

## profile

### create

새 profile 생성.

```bash
# Interactive
gz-git config profile create work

# 플래그로 직접 지정
gz-git config profile create work \
  --provider gitlab \
  --base-url https://gitlab.company.com \
  --token ${WORK_TOKEN} \
  --clone-proto ssh \
  --ssh-port 2224 \
  --parallel 10
```

### use

활성 profile 전환.

```bash
gz-git config profile use work
gz-git config profile use personal
```

### list

모든 profile 목록.

```bash
gz-git config profile list
```

출력:

```
Available profiles:
  default
* work      (active)
  personal
  opensource
```

### show

Profile 상세 정보.

```bash
gz-git config profile show work
```

### delete

Profile 삭제.

```bash
gz-git config profile delete old-profile
```

## show

현재 설정 표시.

```bash
# Project config (.gz-git.yaml)
gz-git config show

# Effective config (모든 레이어 병합)
gz-git config show --effective

# JSON 형식
gz-git config show --format json
```

### Effective Config

5-layer precedence를 적용한 최종 설정:

```bash
gz-git config show --effective
```

출력:

```
Effective Configuration:
========================
provider: gitlab          (from: profile:work)
baseURL: https://gitlab.company.com  (from: profile:work)
token: ***                (from: env:GITLAB_TOKEN)
cloneProto: ssh           (from: project:.gz-git.yaml)
parallel: 10              (from: default)
```

## hierarchy

Config 계층 트리 표시.

```bash
gz-git config hierarchy
```

출력:

```
Config Hierarchy:
=================
~/.config/gz-git/config.yaml (global)
└── ~/.config/gz-git/profiles/work.yaml (active profile)
    └── ~/mydevbox/.gz-git.yaml (project)
        └── ~/mydevbox/subproject/.gz-git.yaml (nested)
```

## 설정 우선순위

높은 우선순위 → 낮은 우선순위:

1. **Command flags** (`--provider gitlab`)
1. **Project config** (`.gz-git.yaml`)
1. **Active profile** (`~/.config/gz-git/profiles/work.yaml`)
1. **Global config** (`~/.config/gz-git/config.yaml`)
1. **Built-in defaults**

## Profile 파일 형식

```yaml
# ~/.config/gz-git/profiles/work.yaml
name: work
provider: gitlab
baseURL: https://gitlab.company.com
token: ${GITLAB_TOKEN}      # 환경변수 참조
cloneProto: ssh
sshPort: 2224
parallel: 10
includeSubgroups: true
subgroupMode: flat

# Command-specific overrides
sync:
  strategy: reset
  maxRetries: 3

branch:
  defaultBranch: develop
  protectedBranches: [main, master]
```

## Project Config 형식

```yaml
# .gz-git.yaml (프로젝트 루트)
profile: work               # 사용할 profile

# Profile 설정 override
sync:
  strategy: pull            # work profile의 reset을 pull로 override
  parallel: 5

metadata:
  team: backend
  repository: https://gitlab.company.com/backend/myproject
```

## 로컬 스캔 제외 (defaults.scan.exclude)

bulk 명령(`push`, `commit`, `clean`, `cleanup branch`, `stash`, `tag`, `exec`,
`switch`, `update`, `pr create` …)은 설정에 선언된 목록이 아니라 **디렉터리를
스캔**해서 대상을 정합니다. 읽기만 하고 절대 쓰지 않아야 할 저장소(vendored 사본,
upstream 미러, 참조용 clone)를 영구히 빼두려면 `defaults.scan.exclude`에
regex를 선언합니다.

```yaml
defaults:
  scan:
    exclude:
      - mirror-repo
      - ^vendor/
```

동작 규칙:

| 규칙                              | 설명                                                                                                                                                |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| 플래그와 **합쳐집니다**           | `--exclude`는 설정을 덮어쓰지 않고 제외 대상을 더합니다. 선언한 제외가 플래그를 잊었다고 사라지면 의미가 없기 때문입니다.                           |
| `--include`로 되살아나지 않습니다 | 스캐너가 include보다 exclude를 먼저 평가합니다. `--include mirror-repo`를 줘도 제외 상태가 유지됩니다.                                              |
| 상위 설정과 **누적됩니다**        | 부모 `.gz-git.yaml`의 제외는 자식이 자기 제외를 선언해도 유지됩니다. 부모가 미러를 막아둔 이유가 자식의 누락으로 풀리면 안 되기 때문입니다.         |
| 매 실행마다 보고됩니다            | 적용 중인 패턴을 stderr에 출력합니다(`-q`로 억제). 조용히 빠지는 저장소가 없도록 하기 위함입니다. `--format json/llm`의 stdout은 오염되지 않습니다. |
| regex가 잘못되면 **실패합니다**   | 패턴이 컴파일되지 않으면 명령이 중단됩니다. 제외가 조용히 풀린 채 실행되는 것보다 안전합니다.                                                       |

```console
$ gz-git push
Excluding repositories matching defaults.scan.exclude: mirror-repo
...
```

> `defaults.filter.include/exclude`는 이것과 **다른 키**입니다. 그쪽은
> `workspace sync`가 forge API에서 받아 온 목록을 거를 때만 쓰이며 로컬 스캔에는
> 영향이 없습니다. [workspace-command.md](workspace-command.md#%EC%A0%81%EC%9A%A9-%EB%B2%94%EC%9C%84--forge-api-%EB%AA%A9%EB%A1%9D%EC%97%90%EB%A7%8C-%EC%A0%81%EC%9A%A9%EB%90%A9%EB%8B%88%EB%8B%A4) 참고.

## 환경변수

Config 파일에서 `${VAR_NAME}` 문법으로 환경변수 참조:

```yaml
token: ${GITLAB_TOKEN}
baseURL: ${GITLAB_URL}
sshKeyPath: ${HOME}/.ssh/id_ed25519_work
```

## 보안

- Profile 파일: 0600 권한 (소유자만 읽기/쓰기)
- Config 디렉토리: 0700 권한
- Token은 환경변수로 관리 권장
- Shell 명령 실행 없음 (`${VAR}` 확장만)

## 사용 예제

### 멀티 환경 설정

```bash
# 1. Profile 생성
gz-git config profile create work \
  --provider gitlab \
  --base-url https://gitlab.company.com \
  --token $WORK_TOKEN

gz-git config profile create personal \
  --provider github \
  --token $GITHUB_TOKEN

# 2. 환경 전환
gz-git config profile use work
gz-git forge from --org backend    # work profile 사용

gz-git config profile use personal
gz-git forge from --org my-projects  # personal profile 사용

# 3. 일회성 override
gz-git forge from --profile work --org backend
```

### 프로젝트별 설정

```bash
# 프로젝트에 .gz-git.yaml 생성
cat > .gz-git.yaml << EOF
profile: work
sync:
  strategy: pull
  parallel: 3
EOF

# 이제 이 디렉토리에서는 work profile + 로컬 override 적용
gz-git workspace sync
```

### 설정 디버깅

```bash
# 현재 적용된 설정 확인
gz-git config show --effective

# 계층 구조 확인
gz-git config hierarchy

# 특정 값이 어디서 오는지 확인
gz-git config show --effective | grep provider
```
