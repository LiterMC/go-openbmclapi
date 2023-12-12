#!/usr/bin/env bash

curdir="$(dirname $0)"

available_platforms=(
	darwin/amd64 darwin/arm64
	linux/386 linux/amd64 linux/arm linux/arm64
	windows/386 windows/amd64 windows/arm windows/arm64
)

outputdir=output

mkdir -p "$outputdir"

export CGO_ENABLED=0

for p in "${available_platforms[@]}"; do
	os=${p%/*}
	arch=${p#*/}
	target="${outputdir}/go-opembmclapi-${os}-${arch}"
	if [[ "$os" == "windows" ]]; then
		target="${target}.exe"
	fi
	echo "Building $target ..."
	GOOS=$os GOARCH=$arch go build -o "$target" "$@" "$curdir" || exit $?
done
