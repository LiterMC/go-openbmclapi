#!/bin/bash

PUBLIC_PREFIX=craftmine/go-openbmclapi
BUILD_PLATFORMS=(linux/arm64 linux/amd64) #

cd $(dirname $0)

[ -n "$TAG" ] || TAG=$(git describe --tags --match v[0-9]* --abbrev=0 2>/dev/null || git log -1 --format="dev-%H")

function build(){
	platform=$1
	fulltag="${PUBLIC_PREFIX}:${TAG}"
	echo
	echo "==> Building $fulltag"
	echo
	DOCKER_BUILDKIT=1 docker build --platform ${platform} \
	 --tag "$fulltag" \
	 --file "Dockerfile" \
	 --build-arg "TAG=$TAG" \
	 . || return $?
	echo
	docker tag "$fulltag" "${PUBLIC_PREFIX}:latest"
	echo "==> Pushing ${fulltag} ${PUBLIC_PREFIX}:latest"
	echo
	(docker push "$fulltag" && docker push "${PUBLIC_PREFIX}:latest") || return $?
	return 0
}

for platform in "${BUILD_PLATFORMS[@]}"; do
	build $platform || exit $?
done
