#!/usr/bin/env bash
set -Eeuo pipefail

readonly TARGET="${1:-alex@192.168.68.62}"
readonly SERVICE="easy-chess-results-api"
readonly INSTALL_PATH="/usr/local/bin/${SERVICE}"

repo_root="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
branch="$(git -C "$repo_root" branch --show-current)"

if [[ -z "$branch" ]]; then
  echo "Cannot deploy from a detached HEAD." >&2
  exit 1
fi

if ! git -C "$repo_root" diff --quiet || ! git -C "$repo_root" diff --cached --quiet; then
  echo "Refusing to pull with uncommitted tracked changes." >&2
  exit 1
fi

echo "Updating $branch from origin..."
git -C "$repo_root" pull --ff-only origin "$branch"

echo "Running tests..."
(cd "$repo_root" && go test ./...)

work_dir="$(mktemp -d)"
remote_binary="/tmp/${SERVICE}.$$"

cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

echo "Building Linux ARM64 binary..."
(cd "$repo_root" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -trimpath -o "$work_dir/$SERVICE" ./cmd/api)

echo "Installing and restarting $SERVICE..."
ssh -o StrictHostKeyChecking=accept-new "$TARGET" \
  "set -e; cat > '$remote_binary'; chmod 0755 '$remote_binary'; sudo install -o root -g root -m 0755 '$remote_binary' '$INSTALL_PATH'; rm -f '$remote_binary'; sudo systemctl restart '$SERVICE'; sudo systemctl is-active --quiet '$SERVICE'; curl -fsS --retry 10 --retry-delay 1 --retry-connrefused http://127.0.0.1:8080/health/ready" \
  < "$work_dir/$SERVICE"

printf '\nDeployment complete: http://%s:8080\n' "${TARGET#*@}"
