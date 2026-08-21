# gz-git cleanup

Git 브랜치 정리 명령어. **기본은 dry-run**이며 `--force`가 있어야 삭제한다.

## 서브커맨드

| 커맨드   | 설명                                                          |
| -------- | ------------------------------------------------------------- |
| `branch` | 브랜치 정리 (`--merged`, `--stale`, `--gone`, `--superseded`) |
| `wizard` | 대화형 정리                                                   |

타입 플래그를 하나 이상 지정해야 한다. `gz-git cleanup branch`만 치면 오류다.

## cleanup branch

```bash
# 미리보기 (삭제 없음)
gz-git cleanup branch --merged
gz-git cleanup branch --stale
gz-git cleanup branch --gone

# 실제 삭제 (dry-run 해제)
gz-git cleanup branch --merged --force

# 벌크, 비대화형
gz-git cleanup branch --merged --force --yes .
```

## 브랜치 타입

| 플래그         | 설명                                         | 감지                                                                      |
| -------------- | -------------------------------------------- | ------------------------------------------------------------------------- |
| `--merged`     | base에 이미 들어간 브랜치                    | 로컬: `git branch --merged`. 원격만 있는 이름: `merge-base --is-ancestor` |
| `--stale`      | N일간 커밋 없음 (기본 30일)                  | 마지막 커밋 날짜. **원격만 있는 이름은 삭제하지 않음**                    |
| `--gone`       | 추적하던 원격이 사라진 로컬 브랜치           | upstream gone                                                             |
| `--superseded` | 머지되지 않았지만 base가 봇 버전을 이미 충족 | go.mod / Actions `uses:` 버전 비교. 봇 이름만. 조상 아님                  |

## 봇 원격 회수

`dependabot/` `renovate/` `github-actions/` 이름만 대상으로 한다. Dependabot PR을 머지하는 `/git:dependabot-merge`와는 다른 일이다.

```bash
gz-git info --audit
gz-git cleanup branch --bots --merged --remote --format json .
gz-git cleanup branch --bots --superseded --remote --format json .
# 사용자가 삭제를 요청한 뒤에만
gz-git cleanup branch --bots --merged --remote --force --yes .
gz-git cleanup branch --bots --superseded --remote --force --yes .
```

| 감사 코드                       | 의미                                                                | Autofix | 삭제                           |
| ------------------------------- | ------------------------------------------------------------------- | ------- | ------------------------------ |
| `REMOTE_BOT_BRANCH_RECLAIMABLE` | tip이 base의 조상                                                   | false   | 사용자 요청 시 `--force --yes` |
| `REMOTE_BOT_BRANCH_SUPERSEDED`  | 머지되지 않았지만 base가 봇 버전을 이미 충족 (버전 비교, 조상 아님) | false   | 사용자 요청 시 `--force --yes` |
| `REMOTE_BOT_BRANCH_PENDING`     | 머지되지 않음. 아직 더 새거나 비교 불가. 열린 PR일 수 있음          | false   | 하지 않음                      |

JSON 스키마: `gz-git.cleanup.branch/v1`.

`--merged --remote`에 `--bots`가 없으면 **머지된 원격 전부**를 지운다. 사람 토픽 브랜치도 포함된다.

## 주요 옵션

| 옵션                                               | 설명                                                             | 기본값    |
| -------------------------------------------------- | ---------------------------------------------------------------- | --------- |
| `--merged` / `--stale` / `--gone` / `--superseded` | 정리 타입 (하나 이상 필수)                                       | 없음      |
| `--bots`                                           | 봇 이름만 (`dependabot/` `renovate/` `github-actions/`)          | false     |
| `--stale-days`                                     | stale 기준 일수                                                  | 30        |
| `-n, --dry-run`                                    | 미리보기                                                         | true      |
| `--force`                                          | 실제 삭제 (dry-run 해제)                                         | false     |
| `-y, --yes`                                        | 벌크 삭제 확인 생략 (비대화형에서 필요)                          | false     |
| `-r, --remote`                                     | `--merged`(조상) 또는 `--superseded`(버전 충족)일 때만 원격 삭제 | false     |
| `--protect`                                        | 추가 보호 이름 (쉼표 구분)                                       | 없음      |
| `--base`                                           | merge 판정 base                                                  | 자동      |
| `--format`                                         | `default`, `compact`, `json`, `llm`                              | `default` |
| `-d, --scan-depth`                                 | 스캔 깊이                                                        | 1         |
| `-j, --parallel`                                   | 병렬 수                                                          | 10        |

`--force`는 `git branch -D`가 아니다. dry-run을 끄는 스위치다.

## 보호 브랜치

내장 목록은 항상 보호된다. cleanup은 `branch.protectedBranches` 설정을 읽지 않는다. 추가는 `--protect`.

- `main`
- `master`
- `develop`
- `development`
- `release/*`
- `hotfix/*`

```bash
gz-git cleanup branch --merged --protect "staging,qa" --force
```

## 원격 삭제

기본은 로컬만. `--remote` (`-r`)는 `--merged`이고 tip이 base 조상일 때, 또는 `--superseded`이고 base가 봇 버전을 이미 충족할 때만 원격을 지운다. `--stale`/`--gone`에는 원격만 있는 이름을 지우지 않는다. 이 명령의 `-r`은 `--remote`이며 `--recursive`가 아니다. `--superseded`는 봇 이름만 대상으로 하며, 삭제는 기존 lease(`--force-with-lease`) 경로다.

```bash
gz-git cleanup branch --merged --remote
gz-git cleanup branch --bots --merged --remote --format json .
gz-git cleanup branch --bots --superseded --remote --format json .
```

원격 `push --delete`는 되돌릴 수 없다. 로컬 삭제는 `git reflog`로 커밋을 찾을 수 있다.

## 예제

### 정기 미리보기 후 삭제

```bash
gz-git cleanup branch --merged --stale --gone .
# 출력을 본 뒤
gz-git cleanup branch --merged --stale --gone --force --yes .
```

### Stale 기준

```bash
gz-git cleanup branch --stale --stale-days 60
gz-git cleanup branch --stale --stale-days 180 --force
```

### CI에서 머지된 브랜치만

```bash
gz-git cleanup branch --merged --force --yes .
```

## 주의

- 타입 플래그 없이 실행하면 실패한다.
- `--merged --remote`만 쓰면 머지된 원격 전부가 대상이다. 봇만 지우려면 `--bots`.
- `develop` 등 내장 보호 이름은 삭제되지 않는다.
- 원격 삭제는 reflog로 복구할 수 없다.
