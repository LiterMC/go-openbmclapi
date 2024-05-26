#!/bin/bash
# OpenBmclAPI (Golang Edition)
# Copyright (C) 2024 Kevin Z <zyxkad@gmail.com>
# All rights reserved
#
#  This program is free software: you can redistribute it and/or modify
#  it under the terms of the GNU Affero General Public License as published
#  by the Free Software Foundation, either version 3 of the License, or
#  (at your option) any later version.
#
#  This program is distributed in the hope that it will be useful,
#  but WITHOUT ANY WARRANTY; without even the implied warranty of
#  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
#  GNU Affero General Public License for more details.
#
#  You should have received a copy of the GNU Affero General Public License
#  along with this program.  If not, see <https://www.gnu.org/licenses/>.

PUBLIC_PREFIX=craftmine/go-openbmclapi
BUILD_PLATFORMS=(linux/arm64 linux/amd64) #

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
