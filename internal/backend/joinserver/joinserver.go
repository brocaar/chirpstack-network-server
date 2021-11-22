package joinserver

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type serverItem struct {
	joinEUI lorawan.EUI64
	client  backend.Client
}

var (
	servers []serverItem
	keks    map[string][]byte

	netID               lorawan.NetID
	resolveJoinEUI      bool
	resolveDomainSuffix string
	defaultServer       string
	defaultCACert       string
	defaultTLSCert      string
	defaultTLSKey       string
	defaultRedisClient  redis.UniversalClient
	defaultAsyncTimeout time.Duration
)

// Setup sets up the joinserver backend.
func Setup(c config.Config) error {
	conf := c.JoinServer
	keks = make(map[string][]byte)

	netID = c.NetworkServer.NetID
	defaultServer = c.JoinServer.Default.Server
	defaultCACert = c.JoinServer.Default.CACert
	defaultTLSCert = c.JoinServer.Default.TLSCert
	defaultTLSKey = c.JoinServer.Default.TLSKey
	resolveJoinEUI = c.JoinServer.ResolveJoinEUI
	resolveDomainSuffix = c.JoinServer.ResolveDomainSuffix
	if c.JoinServer.Default.Async {
		defaultRedisClient = storage.RedisClient()
	}
	defaultAsyncTimeout = c.JoinServer.Default.AsyncTimeout

	for _, s := range conf.Servers {
		var joinEUI lorawan.EUI64
		if err := joinEUI.UnmarshalText([]byte(s.JoinEUI)); err != nil {
			return errors.Wrap(err, "decode joineui error")
		}

		if s.Server == "" {
			s.Server = joinEUIToServer(joinEUI, resolveDomainSuffix)
		}

		var redisClient redis.UniversalClient
		if s.Async {
			redisClient = storage.RedisClient()
		}

		fmt.Println("FOOOO")
		client, err := backend.NewClient(backend.ClientConfig{
			Logger:       log.StandardLogger(),
			SenderID:     c.NetworkServer.NetID.String(),
			ReceiverID:   joinEUI.String(),
			Server:       s.Server,
			CACert:       s.CACert,
			TLSCert:      s.TLSCert,
			TLSKey:       s.TLSKey,
			RedisClient:  redisClient,
			AsyncTimeout: s.AsyncTimeout,
		})
		if err != nil {
			return errors.Wrap(err, "new backend client error")
		}

		servers = append(servers, serverItem{
			joinEUI: joinEUI,
			client:  client,
		})
	}

	for _, k := range conf.KEK.Set {
		kek, err := hex.DecodeString(k.KEK)
		if err != nil {
			return errors.Wrap(err, "decode kek error")
		}

		keks[k.Label] = kek
	}

	return nil
}

// GetClientForJoinEUI returns the backend client for the given JoinEUI.
func GetClientForJoinEUI(joinEUI lorawan.EUI64) (backend.Client, error) {
	// Pre-configured join-servers.
	for _, s := range servers {
		if s.joinEUI == joinEUI {
			return s.client, nil
		}
	}

	// If DNS resolving is enabled, try to resolve the join-server through DNS.
	if resolveJoinEUI {
		client, err := resolveClient(joinEUI)
		if err == nil {
			return client, nil
		}

		log.WithFields(log.Fields{
			"join_eui": joinEUI,
		}).Warning("joinserver: resolving JoinEUI failed, returning default join-server client")
	}

	// Default join-server.
	return getDefaultClient(joinEUI)
}

func resolveClient(joinEUI lorawan.EUI64) (backend.Client, error) {
	server := joinEUIToServer(joinEUI, resolveDomainSuffix)
	serverParsed, err := url.Parse(server)
	if err != nil {
		return nil, errors.Wrap(err, "parse url error")
	}

	_, err = net.LookupIP(serverParsed.Host)
	if err != nil {
		return nil, errors.Wrap(err, "lookup ip error")
	}

	client, err := backend.NewClient(backend.ClientConfig{
		Logger:       log.StandardLogger(),
		SenderID:     netID.String(),
		ReceiverID:   joinEUI.String(),
		Server:       server,
		CACert:       defaultCACert,
		TLSCert:      defaultTLSCert,
		TLSKey:       defaultTLSKey,
		RedisClient:  defaultRedisClient,
		AsyncTimeout: defaultAsyncTimeout,
	})
	if err != nil {
		return nil, errors.Wrap(err, "joinserver: new client error")
	}

	return client, nil
}

func getDefaultClient(joinEUI lorawan.EUI64) (backend.Client, error) {
	defaultClient, err := backend.NewClient(backend.ClientConfig{
		Logger:       log.StandardLogger(),
		SenderID:     netID.String(),
		ReceiverID:   joinEUI.String(),
		Server:       defaultServer,
		CACert:       defaultCACert,
		TLSCert:      defaultTLSCert,
		TLSKey:       defaultTLSKey,
		RedisClient:  defaultRedisClient,
		AsyncTimeout: defaultAsyncTimeout,
	})
	if err != nil {
		return nil, errors.Wrap(err, "joinserver: new default client error")
	}

	return defaultClient, nil
}

// GetKEKKey returns the KEK key for the given label.
func GetKEKKey(label string) ([]byte, error) {
	kek, ok := keks[label]
	if !ok {
		return nil, fmt.Errorf("kek label '%s' is not configured", label)
	}
	return kek, nil
}

func joinEUIToServer(joinEUI lorawan.EUI64, domain string) string {
	nibbles := strings.Split(joinEUI.String(), "")

	for i, j := 0, len(nibbles)-1; i < j; i, j = i+1, j-1 {
		nibbles[i], nibbles[j] = nibbles[j], nibbles[i]
	}

	return "https://" + strings.Join(nibbles, ".") + domain
}
