
FROM ubuntu:latest

RUN mkdir -p /web && mkdir -p /web/work && cd /web &&\
 apt-get update && apt-get install -y curl &&\
 curl -L -o /web/linux-amd64-openbmclapi\
 "https://github.com/KpnmServer/go-openbmclapi/releases/download/v0.6.0-5/linux-amd64-openbmclapi" &&\
 chmod +x /web/linux-amd64-openbmclapi &&\
 echo -e '#!/bin/sh\ncd /web/work;if [ ! -f "./config.json" ];then echo "{\"debug\":false,\"port\":80}" >./config.json;fi;'\
'exec /web/linux-amd64-openbmclapi' >/web/runner.sh &&\
 chmod +x /web/runner.sh

CMD exec /web/runner.sh
