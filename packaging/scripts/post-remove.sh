#!/usr/bin/env bash

NAME=loraserver

function remove_systemd {
	systemctl stop $NAME
	systemctl disable $NAME
	rm -f /lib/systemd/system/$NAME.service
}

function remove_initd {
	/etc/init.d/$NAME stop
	update-rc.d -f $NAME remove
	rm -f /etc/init.d/$NAME
}

which systemctl &>/dev/null
if [[ $? -eq 0 ]]; then
	remove_systemd
else
	remove_initd
fi
