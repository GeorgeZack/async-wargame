#!/bin/sh
# Builds the server for Linux and installs it on HOST over ssh, then
# restarts it. Run from the repository root:
#   deploy/deploy.sh root@game.example.com
# GOARCH defaults to amd64; use GOARCH=arm64 for an ARM server.
set -eu
HOST=${1:?usage: deploy/deploy.sh user@host}
ARCH=${GOARCH:-amd64}
BIN=$(mktemp -d)/async-wargame

command -v go >/dev/null || { echo "go not found: run it as 'mise exec -- deploy/deploy.sh $HOST'" >&2; exit 1; }
echo "testing ..."
go test ./... >/dev/null || { echo "tests failed; not deploying" >&2; exit 1; }
echo "building linux/$ARCH ..."
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags='-s -w' -o "$BIN" .

echo "uploading to $HOST ..."
scp -q "$BIN" "$HOST:/tmp/async-wargame.new"
ssh "$HOST" 'set -e
	install -m 0755 /tmp/async-wargame.new /usr/local/bin/async-wargame
	rm -f /tmp/async-wargame.new
	systemctl daemon-reload
	systemctl restart async-wargame
	sleep 1
	systemctl --no-pager --lines=3 status async-wargame'
rm -rf "$(dirname "$BIN")"
