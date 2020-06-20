package roaming

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

// ErrNoAgreement is returned when the requested agreement could not be found.
var ErrNoAgreement = errors.New("agreement not found")

type agreement struct {
	netID                  lorawan.NetID
	passiveRoaming         bool
	passiveRoamingLifetime time.Duration
	passiveRoamingKEKLabel string
	checkMIC               bool
	client                 backend.Client
}

var (
	roamingEnabled bool
	netID          lorawan.NetID
	agreements     []agreement
	keks           map[string][]byte
)

// Setup configures the roaming package.
func Setup(c config.Config) error {
	netID = c.NetworkServer.NetID
	keks = make(map[string][]byte)

	for _, server := range c.Roaming.Servers {
		roamingEnabled = true

		if server.Server == "" {
			server.Server = fmt.Sprintf("https://%s%s", server.NetID.String(), c.Roaming.ResolveNetIDDomainSuffix)
		}

		log.WithFields(log.Fields{
			"net_id":          server.NetID,
			"passive_roaming": server.PassiveRoaming,
			"server":          server.Server,
		}).Info("roaming: configuring roaming agreement")

		client, err := backend.NewClient(netID.String(), server.NetID.String(), server.Server, server.CACert, server.TLSCert, server.TLSKey)
		if err != nil {
			return errors.Wrapf(err, "new roaming client error for netid: %s", server.NetID)
		}

		agreements = append(agreements, agreement{
			netID:                  server.NetID,
			passiveRoaming:         server.PassiveRoaming,
			passiveRoamingLifetime: server.PassiveRoamingLifetime,
			passiveRoamingKEKLabel: server.PassiveRoamingKEKLabel,
			checkMIC:               server.CheckMIC,
			client:                 client,
		})
	}

	for _, k := range c.Roaming.KEK.Set {
		kek, err := hex.DecodeString(k.KEK)
		if err != nil {
			return errors.Wrap(err, "decode kek error")
		}

		keks[k.Label] = kek
	}

	return nil
}

// IsRoamingDevAddr returns true when the DevAddr does not match the NetID of
// the ChirpStack Network Server configuration. In case roaming is disabled,
// this will always return false.
// Note that enabling roaming -and- using ABP devices can be problematic when
// the ABP DevAddr does not match the NetID.
func IsRoamingDevAddr(devAddr lorawan.DevAddr) bool {
	return roamingEnabled && !devAddr.IsNetID(netID)
}

// GetClientForNetID returns the API client for the given NetID.
func GetClientForNetID(netID lorawan.NetID) (backend.Client, error) {
	for _, a := range agreements {
		if a.netID == netID {
			return a.client, nil
		}
	}

	return nil, ErrNoAgreement
}

// GetPassiveRoamingLifetime returns the passive-roaming lifetime for the
// given NetID.
func GetPassiveRoamingLifetime(netID lorawan.NetID) time.Duration {
	for _, a := range agreements {
		if a.netID == netID {
			return a.passiveRoamingLifetime
		}
	}

	return 0
}

// GetKEKKey returns the KEK key for the given label.
func GetKEKKey(label string) ([]byte, error) {
	kek, ok := keks[label]
	if !ok {
		return nil, fmt.Errorf("kek label '%' is not configured", label)
	}
	return kek, nil
}

// GetPassiveRaomingKEKLabel returns the KEK label for the given NetID or an empty string.
func GetPassiveRaomingKEKLabel(netID lorawan.NetID) string {
	for _, a := range agreements {
		if a.netID == netID {
			return a.passiveRoamingKEKLabel
		}
	}
	return ""
}
