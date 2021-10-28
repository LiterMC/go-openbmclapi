#!/bin/sh

cd $(dirname $0)

if [ ! -f './pid' ]; then
	exit 0
fi

PID="`cat ./pid`"
rm ./pid

if [ "x`ps -o command= $PID`" = 'x./service-linux-go-openbmclapi' ]; then
	kill -s SIGQUIT $PID
fi
