#!/bin/sh
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

docker pull craftmine/go-openbmclapi:latest || {
 echo "[ERROR] Failed to pull docker image 'craftmine/go-openbmclapi:latest'"
 if ! docker images craftmine/go-openbmclapi | grep latest; then
 	echo "Can not found docker image 'craftmine/go-openbmclapi:latest'"
 	exit 1
 fi
}

docker run -d --name my-go-openbmclapi \
	-e CLUSTER_ID=${CLUSTER_ID} \
	-e CLUSTER_SECRET=${CLUSTER_SECRET} \
	-e CLUSTER_PUBLIC_PORT=${CLUSTER_PUBLIC_PORT} \
	-e CLUSTER_IP=${CLUSTER_IP} \
	-v "${PWD}/cache":/opt/openbmclapi/cache \
	-v "${PWD}/data":/opt/openbmclapi/data \
	-v "${PWD}/logs":/opt/openbmclapi/logs \
	-v "${PWD}/config.yaml":/opt/openbmclapi/config.yaml \
	-p ${CLUSTER_PORT}:4000 \
	craftmine/go-openbmclapi:latest
