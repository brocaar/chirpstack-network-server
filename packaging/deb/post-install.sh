#!/usr/bin/env bash

NAME=loraserver
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

# create configuration directory
if [[ ! -d /etc/$NAME ]]; then
	mkdir /etc/$NAME
	chown $DAEMON_USER:$DAEMON_GROUP /etc/$NAME
	chmod 750 /etc/$NAME
fi

# migrate old environment variable based configuration to new format and
# path.
if [[ -f /etc/default/$NAME && ! -f /etc/$NAME/$NAME.toml ]]; then
	set -a
	source /etc/default/$NAME
	loraserver configfile > /etc/$NAME/$NAME.toml
	chown $DAEMON_USER:$DAEMON_GROUP /etc/$NAME/$NAME.toml
	chmod 640 /etc/$NAME/$NAME.toml
	mv /etc/default/$NAME /etc/default/$NAME.backup

	echo -e "\n\n\n"
	echo "-----------------------------------------------------------------------------------------"
	echo "Your configuration file has been migrated to a new location and format!"
	echo "Path: /etc/$NAME/$NAME.toml"
	echo "-----------------------------------------------------------------------------------------"
	echo -e "\n\n\n"
fi

# create example configuration file
if [[ ! -f /etc/$NAME/$NAME.toml ]]; then
	loraserver configfile > /etc/$NAME/$NAME.toml
	chown $DAEMON_USER:$DAEMON_GROUP /etc/$NAME/$NAME.toml
	chmod 640 /etc/$NAME/$NAME.toml
	echo -e "\n\n\n"
	echo "---------------------------------------------------------------------------------"
	echo "A sample configuration file has been copied to: /etc/$NAME/$NAME.toml"
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
