// +build windows

package cmd

import (
	"github.com/brocaar/chirpstack-gateway-bridge/internal/config"
	log "github.com/sirupsen/logrus"
)

func setSyslog() error {
	if config.C.General.LogToSyslog {
		log.Fatal("syslog logging is not supported on Windows")
	}

	return nil
}
