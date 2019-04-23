#!/usr/bin/env bash

NAME=loraserver

function remove_systemd {
	systemctl stop $NAME
	systemctl disable $NAME
	rm -f /lib/systemd/system/$NAME.service
	rm -f /lib/systemd/system/$NAME-mqtt2to3.service
}

function remove_initd {
	/etc/init.d/$NAME stop
	update-rc.d -f $NAME remove
	rm -f /etc/init.d/$NAME
	rm -f /etc/init.d/$NAME-mqtt2to3
}

which systemctl &>/dev/null
if [[ $? -eq 0 ]]; then
	remove_systemd
else
	remove_initd
fi
