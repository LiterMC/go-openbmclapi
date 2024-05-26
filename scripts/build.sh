#!/usr/bin/env bash
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

available_platforms=(
	darwin/amd64 darwin/arm64
	linux/386 linux/amd64 linux/arm linux/arm64
	# windows/386 windows/arm
	# windows/amd64 windows/arm64 # windows builds are at build-windows.bat
)

outputdir=output

mkdir -p "$outputdir"

[ -n "$TAG" ] || TAG=$(git describe --tags --match v[0-9]* --abbrev=0 --candidates=0 2>/dev/null || git log -1 --format="dev-%H")

echo "Detected tag: $TAG"

ldflags="-X 'github.com/LiterMC/go-openbmclapi/internal/build.BuildVersion=$TAG'"

export CGO_ENABLED=0

for p in "${available_platforms[@]}"; do
	os=${p%/*}
	arch=${p#*/}
	target="${outputdir}/go-openbmclapi-${os}-${arch}"
	if [[ "$os" == "windows" ]]; then
		target="${target}.exe"
	fi
	echo "Building $target ..."
	GOOS=$os GOARCH=$arch go build -o "$target" -ldflags "$ldflags" "$@" . || exit $?
done
