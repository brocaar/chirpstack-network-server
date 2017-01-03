#!/usr/bin/env bash

BIN_DIR=/usr/bin
SCRIPT_DIR=/usr/lib/loraserver/scripts
LOG_DIR=/var/log/loraserver
DAEMON_USER=loraserver
DAEMON_GROUP=loraserver

function install_init {
	cp -f $SCRIPT_DIR/init.sh /etc/init.d/loraserver
	chmod +x /etc/init.d/loraserver
	update-rc.d loraserver defaults
}

function install_systemd {
	cp -f $SCRIPT_DIR/loraserver.service /lib/systemd/system/loraserver.service
	systemctl daemon-reload
	systemctl enable loraserver
}

function restart_loraserver {
	echo "Restarting LoRa Server"
	which systemctl &>/dev/null
	if [[ $? -eq 0 ]]; then
		systemctl daemon-reload
		systemctl restart loraserver
	else
		/etc/init.d/loraserver restart || true
	fi	
}

# create loraserver user
id loraserver &>/dev/null
if [[ $? -ne 0 ]]; then
	useradd --system -U -M loraserver -d /bin/false
fi

mkdir -p "$LOG_DIR"
chown loraserver:loraserver "$LOG_DIR"

# add defaults file, if it doesn't exist
if [[ ! -f /etc/default/loraserver ]]; then
	cp $SCRIPT_DIR/default /etc/default/loraserver
	chown loraserver:loraserver /etc/default/loraserver
	chmod 640 /etc/default/loraserver
	echo -e "\n\n\n"
	echo "---------------------------------------------------------------------------------"
	echo "A sample configuration file has been copied to: /etc/default/loraserver"
	echo "After setting the correct values, run the following command to start LoRa Server:"
	echo ""
	which systemctl &>/dev/null
	if [[ $? -eq 0 ]]; then
		echo "$ sudo systemctl start loraserver"
	else
		echo "$ sudo /etc/init.d/loraserver start"
	fi
	echo "---------------------------------------------------------------------------------"
	echo -e "\n\n\n"
fi

# add start script
which systemctl &>/dev/null
if [[ $? -eq 0 ]]; then
	install_systemd
else
	install_init
fi

if [[ -n $2 ]]; then
	restart_loraserver
fi
