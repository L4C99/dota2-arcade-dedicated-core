#!/usr/bin/env bash
# CI only: no Dota, remote hosts, credentials, account creation or sysctl changes.
set -euo pipefail
mode=${1:?check mode required}
logs="$RUNNER_TEMP/d2core-ci-logs"
mkdir -p "$logs"
if [[ "$mode" == prepare ]]; then
  exec > >(tee "$logs/prepare.log") 2>&1
  go version
  expected=$(awk '$1 == "go" {print $2}' "$GITHUB_WORKSPACE/source/go.mod")
  [[ "$(go env GOVERSION)" == "go$expected" ]]
  gcc --version
  git -C "$GITHUB_WORKSPACE/source" rev-parse HEAD
  test -z "$(git -C "$GITHUB_WORKSPACE/source" status --porcelain)"
  python3 "$GITHUB_WORKSPACE/ci-driver/.github/ci/proc-diagnostic.py"
  uid=$(id -u nobody)
  [[ "$uid" != 0 && "$uid" != "$(id -u)" ]]
  root=$(mktemp -d "$RUNNER_TEMP/d2core-ci.XXXXXX")
  # Existing unprivileged account; change only this per-job temporary directory.
  mkdir "$root/source" "$root/home" "$root/tmp" "$root/cache" "$root/mod"
  cp -a "$GITHUB_WORKSPACE/source/." "$root/source/"
  sudo chown -R "$uid:$(id -g nobody)" "$root"
  go_path=$(command -v go)
  printf 'D2_CI_ROOT=%s\nD2_GO=%s\n' "$root" "$go_path" >> "$GITHUB_ENV"
  sudo -u nobody -- python3 "$GITHUB_WORKSPACE/ci-driver/.github/ci/proc-diagnostic.py"
  sudo -u nobody -- env HOME="$root/home" GOTOOLCHAIN=local GOCACHE="$root/cache" GOMODCACHE="$root/mod" TMPDIR="$root/tmp" \
    bash -c 'cd "$1/source"; "$2" mod download && "$2" mod verify && test -z "$(git status --porcelain)"' _ "$root" "$go_path"
  exit 0
fi
case "$mode" in
  test) args=(test ./... -count=1) ;;
  vet) args=(vet ./...) ;;
  build) args=(build -o "$D2_CI_ROOT/d2core" ./cmd/d2core) ;;
  race) args=(test -race ./... -count=1) ;;
  *) echo "unknown check: $mode" >&2; exit 2 ;;
esac
# pipefail propagates every failing check; no continue-on-error or test filtering.
sudo -u nobody -- env HOME="$D2_CI_ROOT/home" GOTOOLCHAIN=local CGO_ENABLED=1 CC=gcc \
  GOCACHE="$D2_CI_ROOT/cache" GOMODCACHE="$D2_CI_ROOT/mod" TMPDIR="$D2_CI_ROOT/tmp" \
  bash -c 'cd "$1/source"; shift; exec "$@"' _ "$D2_CI_ROOT" "$D2_GO" "${args[@]}" 2>&1 | tee "$logs/$mode.log"
