#!/bin/bash

if [ $(id -u) -ne 0 ]; then
	echo -e "\e[31mERROR: Not root user\e[0m"
	exit 1
fi

echo "MIRROR_PREFIX=${MIRROR_PREFIX}"

REPO='LiterMC/go-openbmclapi'
RAW_PREFIX="${MIRROR_PREFIX}https://raw.githubusercontent.com"
RAW_REPO="$RAW_PREFIX/$REPO"
BASE_PATH=/opt/openbmclapi
USERNAME=openbmclapi

if ! systemd --version >/dev/null 2>&1 ; then
	echo -e "\e[31mERROR: Failed to test systemd\e[0m"
	exit 1
fi

if [ ! -d /usr/lib/systemd/system/ ]; then
	echo -e "\e[31mERROR: /usr/lib/systemd/system/ is not exist\e[0m"
	exit 1
fi

if ! id $USERNAME >/dev/null 2>&1; then
	echo -e "\e[34m==> Creating user $USERNAME\e[0m"
	useradd $USERNAME || {
		echo -e "\e[31mERROR: Could not create user $USERNAME\e[0m"
		exit 1
  	}
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

	source="$RAW_REPO/$TARGET_TAG/$file"
	echo -e "\e[34m==> Downloading $source\e[0m"
	tmpf=$(mktemp -t go-openbmclapi.XXXXXXXXXXXX.downloading)
	curl -fsSL -o "$tmpf" "$source" || { rm "$tmpf"; return 1; }
	echo -e "\e[34m==> Downloaded $source\e[0m"
	mv "$tmpf" "$target" || return $?
	echo -e "\e[34m==> Installed to $target\e[0m"
	chown $USERNAME "$target"
	if [ -n "$filemod" ]; then
		chmod "$filemod" "$target" || return $?
	fi
}

echo

if [ -f /usr/lib/systemd/system/go-openbmclapi.service ]; then
	echo -e "\e[33m==> WARN: go-openbmclapi.service is already installed, stopping\e[0m"
	systemctl disable --now go-openbmclapi.service
fi

if [ ! -n "$TARGET_TAG" ]; then
	echo -e "\e[34m==> Fetching latest tag for https://github.com/$REPO\e[0m"
	fetchGithubLatestTag
	TARGET_TAG=$LATEST_TAG
	echo
	echo -e "\e[32m*** go-openbmclapi LATEST TAG: $TARGET_TAG ***\e[0m"
	echo
fi

fetchBlob service/go-openbmclapi.service /usr/lib/systemd/system/go-openbmclapi.service 0644 || exit $?

[ -d "$BASE_PATH" ] || { mkdir -p "$BASE_PATH" && chmod 0755 "$BASE_PATH" && chown $USERNAME "$BASE_PATH"; } || exit $?

# fetchBlob service/start-server.sh "$BASE_PATH/start-server.sh" 0755 || exit $?
# fetchBlob service/stop-server.sh "$BASE_PATH/stop-server.sh" 0755 || exit $?
# fetchBlob service/reload-server.sh "$BASE_PATH/reload-server.sh" 0755 || exit $?

ARCH=$(uname -m)
case "$ARCH" in
    amd64|x86_64)
        ARCH="amd64"
    ;;
    i386|i686)
        ARCH="386"
    ;;
    aarch64|armv8|arm64)
        ARCH="arm64"
    ;;
    armv7l|armv6|armv7)
        ARCH="arm"
    ;;
    *)
        echo -e "\e[31m
Unknown CPU architecture: $ARCH
Please report to https://github.com/LiterMC/go-openbmclapi/issues/new\e[0m"
        exit 1
esac

source="${MIRROR_PREFIX}https://github.com/$REPO/releases/download/$TARGET_TAG/go-openbmclapi-linux-$ARCH"
echo -e "\e[34m==> Downloading $source\e[0m"

curl -fL -o "$BASE_PATH/service-linux-go-openbmclapi" "$source" && \
 chmod 0755 "$BASE_PATH/service-linux-go-openbmclapi" && \
 chown $USERNAME "$BASE_PATH/service-linux-go-openbmclapi" || \
  exit 1

[ -f $BASE_PATH/config.yaml ] || fetchBlob config.yaml $BASE_PATH/config.yaml 0600 || exit $?

echo -e "\e[34m==> Enabling go-openbmclapi.service\e[0m"
systemctl enable go-openbmclapi.service || exit $?

echo -e "
================================ Install successed ================================

  Use 'systemctl start go-openbmclapi.service' to start openbmclapi server
  Use 'systemctl stop go-openbmclapi.service' to stop openbmclapi server
  Use 'systemctl reload go-openbmclapi.service' to reload openbmclapi server configs
  Use 'journalctl -f --output cat -u go-openbmclapi.service' to watch the openbmclapi logs
"
