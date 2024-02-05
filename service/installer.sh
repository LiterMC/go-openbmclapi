#!/bin/bash

if [ $(id -u) -ne 0 ]; then
	echo -e "\e[31mERROR: Not root user\e[0m"
	exit 1
fi

THRESHOLD=0.5
MIRROR_PREFIX=
GITHUB_TIME=$(curl -w "%{time_total}" -s -o /dev/null https://github.com)
if [ $(echo "$GITHUB_TIME > $THRESHOLD" | bc) -eq 1 ]; then
  echo "==> GitHub is too slow, switching to another mirror"
  MIRROR_PREFIX=https://mirror.ghproxy.com/
fi

REPO='LiterMC/go-openbmclapi'
RAW_PREFIX="${MIRROR_PREFIX}https://raw.githubusercontent.com"
RAW_REPO="$RAW_PREFIX/$REPO"
BASE_PATH=/opt/openbmclapi
LATEST_TAG=$1

if ! systemd --version; then
	echo -e "\e[31mERROR: Failed to test systemd\e[0m"
	exit 1
fi

if [ ! -d /usr/lib/systemd/system/ ]; then
	echo -e "\e[31mERROR: /usr/lib/systemd/system/ is not exist\e[0m"
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
	echo -e "\e[34m==> Downloading $source\e[0m"
	tmpf=$(mktemp -t go-openbmclapi.XXXXXXXXXXXX.downloading)
	curl -fsSL -o "$tmpf" "$source" || { rm "$tmpf"; return 1; }
	echo -e "\e[34m==> Downloaded $source\e[0m"
	mv "$tmpf" "$target" || return $?
	echo -e "\e[34m==> Installed to $target\e[0m"
	if [ -n "$filemod" ]; then
		chmod "$filemod" "$target" || return $?
	fi
}

echo

if [ -f /usr/lib/systemd/system/go-openbmclapi.service ]; then
	echo -e "\e[33m==>WARN: go-openbmclapi.service is already installed, stopping\e[0m"
	systemctl stop go-openbmclapi.service
	systemctl disable go-openbmclapi.service
fi

if [ ! -n "$LATEST_TAG" ]; then
	echo -e "\e[34m==> Fetching latest tag for https://github.com/$REPO\e[0m"
	fetchGithubLatestTag
	echo
	echo -e "\e[32m*** go-openbmclapi LATEST TAG: $LATEST_TAG ***\e[0m"
	echo
fi

fetchBlob service/go-openbmclapi.service /usr/lib/systemd/system/go-openbmclapi.service 0644 || exit $?

[ -d "$BASE_PATH" ] || { mkdir -p "$BASE_PATH" && chmod 0755 "$BASE_PATH"; } || exit $?

# fetchBlob service/start-server.sh "$BASE_PATH/start-server.sh" 0755 || exit $?
# fetchBlob service/stop-server.sh "$BASE_PATH/stop-server.sh" 0755 || exit $?
# fetchBlob service/reload-server.sh "$BASE_PATH/reload-server.sh" 0755 || exit $?

latest_src="https://github.com/$REPO/releases/download/$LATEST_TAG"

case "`uname -m`" in
    x86_64)
        GOARCH="amd64"
    ;;
    i386|i686)
        GOARCH="386"
    ;;
    aarch64|armv8)
        GOARCH="arm64"
    ;;
    armv7l|armv6|armv7)
        GOARCH="arm"
    ;;
    *)
        echo -e "\e[31mUnknown CPU architecture: `uname -m`\e[0m"
        exit 1

source="${MIRROR_PREFIX}$latest_src/go-openbmclapi-linux-$arch"
echo -e "\e[34m==> Downloading $source\e[0m"
curl -fL -o "$BASE_PATH/service-linux-go-openbmclapi" "$source"

echo -e "\e[34m==> Add user openbmclapi and setting privilege\e[0m"
useradd openbmclapi
mkdir $BASE_PATH/cache $BASE_PATH/data
curl -fL -o "$BASE_PATH/config.yaml" "${RAW_REPO}/HEAD/config.yaml"
chown -R openbmclapi:openbmclapi $BASE_PATH
chmod 0755 "$BASE_PATH/service-linux-go-openbmclapi" || exit $?


echo -e "\e[34m==> Enable go-openbmclapi.service\e[0m"
systemctl enable go-openbmclapi.service || exit $?

echo -e "
================================ Install successed ================================

  Use 'systemctl start go-openbmclapi.service' to start openbmclapi server
  Use 'systemctl stop go-openbmclapi.service' to stop openbmclapi server
  Use 'systemctl reload go-openbmclapi.service' to reload openbmclapi server configs
  Use 'journalctl -f --output cat -u go-openbmclapi.service' to watch the openbmclapi logs
"
