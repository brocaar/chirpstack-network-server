#!/usr/bin/env bash

# map GOARCH to ARCH
case $GOARCH in
	'amd64') ARCH="x86_64"
		;;
	'386') ARCH="i386"
		;;
	'arm') ARCH="armhf"
		;;
	*)
		echo "Unknown target $GOARCH"
		exit 1
		;;
esac

# validate TARGET
case $TARGET in
	'deb') DEB_WANTED="deb"
		;;
	*)
		echo "Unknown target distribution $TARGET"
		exit 1
		;;
esac

NAME=loraserver
BIN_DIR=/usr/bin
SCRIPT_DIR=/usr/lib/$NAME/scripts
TMP_WORK_DIR=`mktemp -d`
LOGROTATE_DIR=/etc/logrotate.d

POSTINSTALL_SCRIPT=$TARGET/post-install.sh
PREINSTALL_SCRIPT=$TARGET/pre-install.sh
POSTUNINSTALL_SCRIPT=$TARGET/post-uninstall.sh

LICENSE=MIT
VERSION=`git describe --always`
URL=https://docs.loraserver.io/$NAME/
MAINTAINER=info@brocaar.com
VENDOR="LoRa Server project"
DESCRIPTION="LoRaWAN network-server"
DIST_FILE_PATH="../dist/tar/${NAME}_${VERSION}_linux_${GOARCH}.tar.gz"
DEB_FILE_PATH="../dist/deb"

COMMON_FPM_ARGS="\
	--log error \
	-C $TMP_WORK_DIR \
	--url $URL \
	--license $LICENSE \
	--maintainer $MAINTAINER \
	--after-install $POSTINSTALL_SCRIPT \
	--before-install $PREINSTALL_SCRIPT \
	--after-remove $POSTUNINSTALL_SCRIPT \
	--architecture $ARCH \
	--name $NAME \
	--version $VERSION"

if [ ! -f $DIST_FILE_PATH ]; then
	echo "Dist file $DIST_FILE_PATH does not exist"
	exit 1
fi


# make temp dirs
mkdir -p $TMP_WORK_DIR/$BIN_DIR
mkdir -p $TMP_WORK_DIR/$SCRIPT_DIR
mkdir -p $TMP_WORK_DIR/$LOGROTATE_DIR

# unpack pre-compiled binary
tar -zxf $DIST_FILE_PATH -C $TMP_WORK_DIR/$BIN_DIR

# copy scripts
cp $TARGET/init.sh $TMP_WORK_DIR/$SCRIPT_DIR
cp $TARGET/$NAME.service $TMP_WORK_DIR/$SCRIPT_DIR
cp $TARGET/default $TMP_WORK_DIR/$SCRIPT_DIR
cp $TARGET/logrotate $TMP_WORK_DIR/$LOGROTATE_DIR/$NAME

if [ -n "$DEB_WANTED" ]; then
	fpm -s dir -t deb $COMMON_FPM_ARGS --vendor "$VENDOR" --description "$DESCRIPTION" .
	if [ $? -ne 0 ]; then
		echo "Failed to create Debian package -- aborting."
		exit 1
	fi
	mkdir -p ../dist/deb
	mv *.deb ../dist/deb
	echo "Debian package created successfully."
fi
