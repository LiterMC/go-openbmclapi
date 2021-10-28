#!/bin/sh

cd "$(dirname "$0")"

if [ $(id -u) -ne 0 ]; then
	read -p 'ERROR: You are not root user, are you sure to continue?(Y/n)' Y
	echo
	[ "x$Y" = "xY" ] || exit 1
fi

if ! systemd --version; then
	echo "ERROR: Failed to test systemd"
	exit 1
fi

if [ ! -d /usr/lib/systemd/system/ ]; then
	echo 'ERROR: /usr/lib/systemd/system/ are not exist'
	exit
fi

([ -d /var/openbmclapi ] || mkdir -p /var/openbmclapi) || exit $?

(curl -L -o ./service-linux-go-openbmclapi "https://github.com/KpnmServer/go-openbmclapi/releases/download/v0.6.0-6/linux-amd64-openbmclapi" &&\
 cp ./service-linux-go-openbmclapi /var/openbmclapi/service-linux-go-openbmclapi && chmod 0744 /var/openbmclapi/service-linux-go-openbmclapi) || exit $?
(cp ./go-openbmclapi.service /usr/lib/systemd/system/go-openbmclapi.service) || exit $?
(cp ./start-server.sh /var/openbmclapi/start-server.sh && chmod 0744 /var/openbmclapi/start-server.sh) || exit $?
(cp ./stop-server.sh /var/openbmclapi/stop-server.sh && chmod 0744 /var/openbmclapi/stop-server.sh) || exit $?
(cp ./reload-server.sh /var/openbmclapi/reload-server.sh && chmod 0744 /var/openbmclapi/reload-server.sh) || exit $?

systemctl enable go-openbmclapi || exit $?

echo
echo "========Install success========"
echo "Use 'systemctl strat go-openbmclapi' to start bmclapi node server"
