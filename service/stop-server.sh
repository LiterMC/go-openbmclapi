#!/bin/sh

cd $(dirname $0)

if [ ! -f './pid' ]; then
	exit 0
fi

PID="`cat ./pid`"
rm ./pid

if [ "`ps -o command= $PID`" = './service-linux-go-openbmclapi' ]; then
	kill -s QUIT $PID
fi

exit 0
