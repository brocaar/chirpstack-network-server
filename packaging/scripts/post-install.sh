#!/usr/bin/env bash

NAME=loraserver
BIN_DIR=/usr/bin
SCRIPT_DIR=/usr/lib/loraserver/scripts
LOG_DIR=/var/log/loraserver
DAEMON_USER=loraserver
DAEMON_GROUP=loraserver

function install_init {
	cp -f $SCRIPT_DIR/$NAME.init /etc/init.d/$NAME
	cp -f $SCRIPT_DIR/$NAME-mqtt2to3.init /etc/init.d/$NAME-mqtt2to3
	chmod +x /etc/init.d/$NAME
	chmod +x /etc/init.d/$NAME-mqtt2to3
	update-rc.d $NAME defaults
}

function install_systemd {
	cp -f $SCRIPT_DIR/$NAME.service /lib/systemd/system/$NAME.service
	cp -f $SCRIPT_DIR/$NAME-mqtt2to3.service /lib/systemd/system/$NAME-mqtt2to3.service
	systemctl daemon-reload
	systemctl enable $NAME
}

function restart_service {
	echo "Restarting $NAME"
	which systemctl &>/dev/null
	if [[ $? -eq 0 ]]; then
		systemctl daemon-reload
		systemctl restart $NAME
	else
		/etc/init.d/$NAME restart || true
	fi	
}

# create user
id $DAEMON_USER &>/dev/null
if [[ $? -ne 0 ]]; then
	useradd --system -U -M $DAEMON_USER -s /bin/false -d /etc/$NAME
fi

mkdir -p "$LOG_DIR"
chown $DAEMON_USER:$DAEMON_GROUP "$LOG_DIR"

# create configuration directory
if [[ ! -d /etc/$NAME ]]; then
	mkdir /etc/$NAME
	chown $DAEMON_USER:$DAEMON_GROUP /etc/$NAME
	chmod 750 /etc/$NAME
fi

# create example configuration file
if [[ ! -f /etc/$NAME/$NAME.toml ]]; then
	$NAME configfile > /etc/$NAME/$NAME.toml
	chown $DAEMON_USER:$DAEMON_GROUP /etc/$NAME/$NAME.toml
	chmod 640 /etc/$NAME/$NAME.toml
	echo -e "\n\n\n"
	echo "---------------------------------------------------------------------------------"
	echo "A sample configuration file has been copied to: /etc/$NAME/$NAME.toml"
	echo "After setting the correct values, run the following command to start $NAME:"
	echo ""
	which systemctl &>/dev/null
	if [[ $? -eq 0 ]]; then
		echo "$ sudo systemctl start $NAME"
	else
		echo "$ sudo /etc/init.d/$NAME start"
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
	restart_service
fi
