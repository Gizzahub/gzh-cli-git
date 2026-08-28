#!/bin/bash
# test-install-path-audit.sh: install-path-audit.sh 가 실패해야 할 때 실제로 실패하는지 검증한다
# 용도: 감사 스크립트를 합성 PATH 위에서 돌린다. 실환경은 대개 깨끗하므로 "오늘 통과했다" 는
#       그 감사가 살아 있다는 증거가 되지 못한다. 그림자·중복·회수·신원미확인·판정불가를
#       각각 만들어 놓고 판정과 종료코드를 잰다.
# 사용법: test-install-path-audit.sh

set -u

here=$(cd "$(dirname "$0")" && pwd -P)
audit="$here/install-path-audit.sh"
tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/gz-git-audit.XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT

failures=0
BASE_PATH=/usr/bin:/bin:/usr/sbin:/sbin
AUDIT_BASH_ENV=/dev/null
AUDIT_ENV=/dev/null

# 자기를 'gz-git' 이라고 밝히는 가짜. 감사는 절대경로로 --version 을 물어 신원을 확인한다.
make_fake() {
	mkdir -p "$1"
	cat >"$1/gz-git" <<FAKE
#!/bin/bash
echo "gz-git version $2"
FAKE
	chmod +x "$1/gz-git"
}

# 이름만 같고 자기를 밝히지 않는 남의 파일.
make_impostor() {
	mkdir -p "$1"
	cat >"$1/gz-git" <<'IMPOSTOR'
#!/bin/bash
echo "some other tool 9.9"
IMPOSTOR
	chmod +x "$1/gz-git"
}

check() {
	name=$1 want_rc=$2 want_text=$3 got_rc=$4 got_out=$5
	if [ "$got_rc" != "$want_rc" ]; then
		echo "FAIL [$name] rc: want=$want_rc got=$got_rc" >&2
		printf '%s\n' "$got_out" | sed 's/^/       /' >&2
		failures=$((failures + 1))
		return
	fi
	if [ -n "$want_text" ] && ! printf '%s' "$got_out" | grep -Fq "$want_text"; then
		echo "FAIL [$name] output missing: $want_text" >&2
		printf '%s\n' "$got_out" | sed 's/^/       /' >&2
		failures=$((failures + 1))
		return
	fi
	echo "ok   [$name]"
}

# 1) 깨끗한 환경 — 설치본만 PATH 에 있다.
good="$tmpdir/good bin"
make_fake "$good" 0.7.0

# A caller's non-interactive shell startup must not rewrite the synthetic PATH.
# Without the per-invocation BASH_ENV/ENV isolation below, a workstation-level
# BASH_ENV can expose and even let a RECLAIM case delete a real installed binary.
ambient="$tmpdir/ambient bin"
make_fake "$ambient" 9.9.0
host_bash_env="$tmpdir/host-bash-env"
printf 'export PATH=%q:$PATH\n' "$ambient" >"$host_bash_env"
export BASH_ENV="$host_bash_env"
export ENV="$host_bash_env"

out=$(BASH_ENV="$AUDIT_BASH_ENV" ENV="$AUDIT_ENV" PATH="$good:$BASE_PATH" "$audit" gz-git "$good/gz-git" 2>&1)
check clean 0 "install-path-audit: OK" "$?" "$out"

# 2) 그림자 — 설치본보다 PATH 앞에 다른 파일이 있다. 설치는 성공했지만 실행되지 않는다.
#    앞선 파일의 버전을 설치본과 **같게** 둔다. 다르게 두면 중복 판정이 같은 실패를
#    만들어내서 그림자 검사를 무력화해도 이 케이스가 통과해 버린다(돌연변이로 확인됨).
same_ver_shadow="$tmpdir/same version shadow bin"
make_fake "$same_ver_shadow" 0.7.0
out=$(BASH_ENV="$AUDIT_BASH_ENV" ENV="$AUDIT_ENV" PATH="$same_ver_shadow:$good:$BASE_PATH" "$audit" gz-git "$good/gz-git" 2>&1)
check shadow 1 "방금 설치한 파일이 실행되지 않는다" "$?" "$out"

shadow="$tmpdir/shadow bin"
make_fake "$shadow" 0.6.1

# 3) 중복 — 다른 버전이 뒤에 있다. 지금은 무해하지만 PATH 순서가 바뀌면 2번이 된다.
out=$(BASH_ENV="$AUDIT_BASH_ENV" ENV="$AUDIT_ENV" PATH="$good:$shadow:$BASE_PATH" "$audit" gz-git "$good/gz-git" 2>&1)
check duplicate 1 "다른 버전의 'gz-git' 가 PATH 에 있다" "$?" "$out"

# 4) WARN_ONLY — 같은 결함을 보고하되 실패시키지 않는다.
out=$(BASH_ENV="$AUDIT_BASH_ENV" ENV="$AUDIT_ENV" INSTALL_AUDIT_WARN_ONLY=1 PATH="$good:$shadow:$BASE_PATH" "$audit" gz-git "$good/gz-git" 2>&1)
check warn-only 0 "install-path-audit: FAIL" "$?" "$out"

# 5) 신원 미확인 — 이름만 같은 남의 파일은 RECLAIM 이 켜져 있어도 삭제하지 않는다.
impostor="$tmpdir/impostor bin"
make_impostor "$impostor"
out=$(BASH_ENV="$AUDIT_BASH_ENV" ENV="$AUDIT_ENV" RECLAIM=1 PATH="$good:$impostor:$BASE_PATH" "$audit" gz-git "$good/gz-git" 2>&1)
rc=$?
check impostor-not-deleted 0 "신원이 확인되지 않는 파일이다" "$rc" "$out"
if [ ! -f "$impostor/gz-git" ]; then
	echo "FAIL [impostor-not-deleted] 남의 파일이 삭제됐다" >&2
	failures=$((failures + 1))
fi

# 6) 회수 — 신원이 확인된 중복만 삭제하고 통과로 돌아선다.
reclaim="$tmpdir/reclaim bin"
make_fake "$reclaim" 0.6.1
out=$(BASH_ENV="$AUDIT_BASH_ENV" ENV="$AUDIT_ENV" RECLAIM=1 PATH="$good:$reclaim:$BASE_PATH" "$audit" gz-git "$good/gz-git" 2>&1)
rc=$?
check reclaim 0 "회수함: 삭제 완료" "$rc" "$out"
if [ -f "$reclaim/gz-git" ]; then
	echo "FAIL [reclaim] 중복본이 남아 있다" >&2
	failures=$((failures + 1))
fi
if [ ! -f "$ambient/gz-git" ]; then
	echo "FAIL [ambient-not-deleted] BASH_ENV 가 노출한 외부 바이너리가 삭제됐다" >&2
	failures=$((failures + 1))
fi

# 7) 판정 불가 — 설치 위치가 PATH 에 없다. 통과가 아니라 SKIP 이어야 한다.
#    make test-install 이 임시 BINDIR 로 install 을 재귀 호출하는 경로가 여기다.
off="$tmpdir/off path bin"
make_fake "$off" 0.7.0
out=$(BASH_ENV="$AUDIT_BASH_ENV" ENV="$AUDIT_ENV" PATH="$BASE_PATH" "$audit" gz-git "$off/gz-git" 2>&1)
check off-path 0 "install-path-audit: SKIP" "$?" "$out"

if [ "$failures" -ne 0 ]; then
	echo "install-path-audit tests: $failures failed" >&2
	exit 1
fi
echo "install-path-audit tests: all passed"
