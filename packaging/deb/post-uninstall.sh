#!/usr/bin/env bash

function remove_systemd {
	systemctl stop loraserver
	systemctl disable loraserver
	rm -f /lib/systemd/system/loraserver.service
}

function remove_initd {
	/etc/init.d/loraserver stop
	update-rc.d -f loraserver remove
	rm -f /etc/init.d/loraserver
}

which systemctl &>/dev/null
if [[ $? -eq 0 ]]; then
	remove_systemd
else
	remove_initd
fi
