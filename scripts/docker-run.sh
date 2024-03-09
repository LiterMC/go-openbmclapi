#!/bin/sh

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
