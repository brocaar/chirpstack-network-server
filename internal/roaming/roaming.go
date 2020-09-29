package roaming

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
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
	server                 string
	client                 backend.Client
}

var (
	resolveNetIDDomainSuffix string
	roamingEnabled           bool
	netID                    lorawan.NetID
	agreements               []agreement
	keks                     map[string][]byte

	defaultEnabled                bool
	defaultPassiveRoaming         bool
	defaultPassiveRoamingLifetime time.Duration
	defaultPassiveRoamingKEKLabel string
	defaultAsync                  bool
	defaultAsyncTimeout           time.Duration
	defaultServer                 string
	defaultCACert                 string
	defaultTLSCert                string
	defaultTLSKey                 string
)

// Setup configures the roaming package.
func Setup(c config.Config) error {
	resolveNetIDDomainSuffix = c.Roaming.ResolveNetIDDomainSuffix
	netID = c.NetworkServer.NetID
	keks = make(map[string][]byte)
	agreements = []agreement{}

	defaultEnabled = c.Roaming.Default.Enabled
	defaultPassiveRoaming = c.Roaming.Default.PassiveRoaming
	defaultPassiveRoamingLifetime = c.Roaming.Default.PassiveRoamingLifetime
	defaultPassiveRoamingKEKLabel = c.Roaming.Default.PassiveRoamingKEKLabel
	defaultAsync = c.Roaming.Default.Async
	defaultAsyncTimeout = c.Roaming.Default.AsyncTimeout
	defaultServer = c.Roaming.Default.Server
	defaultCACert = c.Roaming.Default.CACert
	defaultTLSCert = c.Roaming.Default.TLSCert
	defaultTLSKey = c.Roaming.Default.TLSKey

	if defaultEnabled {
		roamingEnabled = true
	}

	for _, server := range c.Roaming.Servers {
		roamingEnabled = true

		if server.Server == "" {
			server.Server = fmt.Sprintf("https://%s%s", server.NetID.String(), resolveNetIDDomainSuffix)
		}

		log.WithFields(log.Fields{
			"net_id":                   server.NetID,
			"passive_roaming":          server.PassiveRoaming,
			"passive_roaming_lifetime": server.PassiveRoamingLifetime,
			"server":                   server.Server,
			"async":                    server.Async,
			"async_timeout":            server.AsyncTimeout,
		}).Info("roaming: configuring roaming agreement")

		var redisClient redis.UniversalClient
		if server.Async {
			redisClient = storage.RedisClient()
		}

		client, err := backend.NewClient(backend.ClientConfig{
			Logger:       log.StandardLogger(),
			SenderID:     netID.String(),
			ReceiverID:   server.NetID.String(),
			Server:       server.Server,
			CACert:       server.CACert,
			TLSCert:      server.TLSCert,
			TLSKey:       server.TLSKey,
			AsyncTimeout: server.AsyncTimeout,
			RedisClient:  redisClient,
		})
		if err != nil {
			return errors.Wrapf(err, "new roaming client error for netid: %s", server.NetID)
		}

		agreements = append(agreements, agreement{
			netID:                  server.NetID,
			passiveRoaming:         server.PassiveRoaming,
			passiveRoamingLifetime: server.PassiveRoamingLifetime,
			passiveRoamingKEKLabel: server.PassiveRoamingKEKLabel,
			client:                 client,
			server:                 server.Server,
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
func GetClientForNetID(clientNetID lorawan.NetID) (backend.Client, error) {
	for _, a := range agreements {
		if a.netID == clientNetID {
			return a.client, nil
		}
	}

	if defaultEnabled {
		var server string
		if defaultServer == "" {
			server = fmt.Sprintf("https://%s%s", clientNetID, resolveNetIDDomainSuffix)
		} else {
			server = defaultServer
		}

		log.WithFields(log.Fields{
			"net_id":                   clientNetID,
			"passive_roaming":          defaultPassiveRoaming,
			"passive_roaming_lifetime": defaultPassiveRoamingLifetime,
			"server":                   server,
			"async":                    defaultAsync,
			"async_timeout":            defaultAsyncTimeout,
		}).Info("roaming: configuring roaming agreement using default server")

		var redisClient redis.UniversalClient
		if defaultAsync {
			redisClient = storage.RedisClient()
		}

		client, err := backend.NewClient(backend.ClientConfig{
			Logger:       log.StandardLogger(),
			SenderID:     netID.String(),
			ReceiverID:   clientNetID.String(),
			Server:       server,
			CACert:       defaultCACert,
			TLSCert:      defaultTLSCert,
			TLSKey:       defaultTLSKey,
			AsyncTimeout: defaultAsyncTimeout,
			RedisClient:  redisClient,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "new roaming client error for netid: %s", clientNetID)
		}
		return client, nil
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

	if defaultEnabled {
		return defaultPassiveRoamingLifetime
	}

	return 0
}

// GetKEKKey returns the KEK key for the given label.
func GetKEKKey(label string) ([]byte, error) {
	kek, ok := keks[label]
	if !ok {
		return nil, fmt.Errorf("kek label '%s' is not configured", label)
	}
	return kek, nil
}

// GetPassiveRoamingKEKLabel returns the KEK label for the given NetID or an empty string.
func GetPassiveRoamingKEKLabel(netID lorawan.NetID) string {
	for _, a := range agreements {
		if a.netID == netID {
			return a.passiveRoamingKEKLabel
		}
	}

	if defaultEnabled {
		return defaultPassiveRoamingKEKLabel
	}

	return ""
}

// GetNetIDsForDevAddr returns the NetIDs matching the given DevAddr.
func GetNetIDsForDevAddr(devAddr lorawan.DevAddr) []lorawan.NetID {
	var out []lorawan.NetID

	for i := range agreements {
		a := agreements[i]
		if devAddr.IsNetID(a.netID) && a.passiveRoaming {
			out = append(out, a.netID)
		}
	}

	return out
}
