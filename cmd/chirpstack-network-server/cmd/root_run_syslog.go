// +build !windows

package cmd

import (
	"log/syslog"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	lsyslog "github.com/sirupsen/logrus/hooks/syslog"

	"github.com/brocaar/chirpstack-network-server/internal/config"
)

func setSyslog() error {
	if !config.C.General.LogToSyslog {
		return nil
	}

	var prio syslog.Priority

	switch log.StandardLogger().Level {
	case log.DebugLevel:
		prio = syslog.LOG_USER | syslog.LOG_DEBUG
	case log.InfoLevel:
		prio = syslog.LOG_USER | syslog.LOG_INFO
	case log.WarnLevel:
		prio = syslog.LOG_USER | syslog.LOG_WARNING
	case log.ErrorLevel:
		prio = syslog.LOG_USER | syslog.LOG_ERR
	case log.FatalLevel:
		prio = syslog.LOG_USER | syslog.LOG_CRIT
	case log.PanicLevel:
		prio = syslog.LOG_USER | syslog.LOG_CRIT
	}

	hook, err := lsyslog.NewSyslogHook("", "", prio, "chirpstack-network-server")
	if err != nil {
		return errors.Wrap(err, "get syslog hook error")
	}

	log.AddHook(hook)

	return nil
}
