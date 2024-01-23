#!/bin/bash

REPO='LiterMC/go-openbmclapi'
RAW_PREFIX='https://raw.githubusercontent.com'
RAW_REPO="$RAW_PREFIX/$REPO"
BASE_PATH=/opt/openbmclapi


if [ $(id -u) -ne 0 ]; then
	read -p 'ERROR: You are not root user, are you sure to continue?(y/N) ' Y
	echo
	[ "$Y" = "Y" ] || [ "$Y" = "y" ] || exit 1
fi

if ! systemd --version; then
	echo "ERROR: Failed to test systemd"
	exit 1
fi

if [ ! -d /usr/lib/systemd/system/ ]; then
	echo 'ERROR: /usr/lib/systemd/system/ are not exist'
	exit 1
fi

function fetchGithubLatestTag(){
	prefix="location: https://github.com/$REPO/releases/tag/"
	location=$(curl -fsSI "https://github.com/$REPO/releases/latest" | grep "$prefix" | tr -d "\r")
	[ $? = 0 ] || return 1
	export LATEST_TAG="${location#${prefix}}"
}

function fetchBlob(){
	file=$1
	target=$2
	filemod=$3

	source="$RAW_REPO/$LATEST_TAG/$file"
	echo "==> Downloading $source"
	tmpf=$(mktemp -t go-openbmclapi.XXXXXXXXXXXX.downloading)
	curl -fsSL -o "$tmpf" "$source" || { rm "$tmpf"; return 1; }
	echo "==> Downloaded $source"
	mv "$tmpf" "$target" || return $?
	echo "==> Installed to $target"
	if [ -n "$filemod" ]; then
		chmod "$filemod" "$target" || return $?
	fi
}

if [ -f /usr/lib/systemd/system/go-openbmclapi.service ]; then
	echo 'WARN: go-openbmclapi.service is already installed, stopping'
	systemctl stop go-openbmclapi.service
	systemctl disable go-openbmclapi.service
fi

LATEST_TAG=$1

if [ ! -n "$LATEST_TAG" ]; then
	echo "==> Fetching latest tag for https://github.com/$REPO"
	fetchGithubLatestTag
	echo
	echo "*** go-openbmclapi LATEST TAG: $LATEST_TAG ***"
	echo
fi

fetchBlob service/go-openbmclapi.service /usr/lib/systemd/system/go-openbmclapi.service 0644 || exit $?

[ -d "$BASE_PATH" ] || { mkdir -p "$BASE_PATH" && chmod 0755 "$BASE_PATH"; } || exit $?

# fetchBlob service/start-server.sh "$BASE_PATH/start-server.sh" 0755 || exit $?
# fetchBlob service/stop-server.sh "$BASE_PATH/stop-server.sh" 0755 || exit $?
# fetchBlob service/reload-server.sh "$BASE_PATH/reload-server.sh" 0755 || exit $?

latest_src="https://github.com/$REPO/releases/download/$LATEST_TAG"

arch=$(uname -m)
[ "$arch" = 'x86_64' ] && arch=amd64

source="$latest_src/go-opembmclapi-linux-$arch"
echo "==> Downloading $source"
if ! curl -fL -o "$BASE_PATH/service-linux-go-openbmclapi" "$source"; then
	source="$latest_src/go-opembmclapi-linux-amd64"
	echo "==> Downloading fallback binary $source"
	curl -fL -o "$BASE_PATH/service-linux-go-openbmclapi" "$source" || exit $?
fi
chmod 0755 "$BASE_PATH/service-linux-go-openbmclapi" || exit $?


echo "==> Enable go-openbmclapi.service"
systemctl enable go-openbmclapi.service || exit $?

echo "
================================ Install successed ================================

  Use 'systemctl start go-openbmclapi.service' to start openbmclapi server
  Use 'systemctl stop go-openbmclapi.service' to stop openbmclapi server
  Use 'systemctl reload go-openbmclapi.service' to reload openbmclapi server configs
  Use 'journalctl -f --output cat -u go-openbmclapi.service' to watch the openbmclapi logs
"
