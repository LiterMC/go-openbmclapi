#!/bin/sh

cd $(dirname $0)

if [ ! -f ./config.json ];then
	echo '{"debug":false,"port":80}' >./config.json
fi

PID=$$
echo -n "${PID}" >./pid
exec './service-linux-go-openbmclapi'
