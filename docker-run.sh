#!/bin/sh

cd "$(dirname "$0")"

docker pull kupen/go-openbmclapi:latest || {
 echo "[ERROR] Failed to pull docker image 'kupen/go-openbmclapi:latest'"
 if ! docker images kupen/go-openbmclapi | grep latest; then
 	echo "Can not found docker image 'kupen/go-openbmclapi:latest'"
 	exit 1
 fi
}

docker run -d --name my-go-openbmclapi \
	-e CLUSTER_ID=${CLUSTER_ID}\
	-e CLUSTER_SECRET=${CLUSTER_SECRET}\
	-e CLUSTER_PUBLIC_PORT=${CLUSTER_PUBLIC_PORT}\
	-e CLUSTER_IP=${CLUSTER_IP}\
	-v "${PWD}/cache":/web/work/cache\
	-v "${PWD}/logs":/web/work/logs\
	-v "${PWD}/config.json":/web/work/config.json\
	-p ${CLUSTER_PORT}:80\
	kupen/go-openbmclapi:latest
