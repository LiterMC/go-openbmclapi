#!/usr/bin/env bash

curdir="$(dirname $0)"

available_platforms=(
	darwin/amd64 darwin/arm64
	linux/386 linux/amd64 linux/arm linux/arm64
	windows/386 windows/amd64 windows/arm windows/arm64
)

outputdir=output

mkdir -p "$outputdir"

[ -n "$TAG" ] || TAG=$(git describe --tags --match v[0-9]* --abbrev=0 --candidates=0 2>/dev/null || git log -1 --format="dev-%H")

echo "Detected tag: $TAG"

ldflags="-X 'main.BuildVersion=$TAG'"

export CGO_ENABLED=0

for p in "${available_platforms[@]}"; do
	os=${p%/*}
	arch=${p#*/}
	target="${outputdir}/go-openbmclapi-${os}-${arch}"
	if [[ "$os" == "windows" ]]; then
		target="${target}.exe"
	fi
	echo "Building $target ..."
	GOOS=$os GOARCH=$arch go build -o "$target" -ldflags "$ldflags" "$@" "$curdir" || exit $?
done
