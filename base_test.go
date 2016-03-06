package loraserver

import (
	"io/ioutil"
	golog "log"
	"os"

	"github.com/DavidHuie/gomigrate"
	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
)

func init() {
	log.SetLevel(log.ErrorLevel)
	golog.SetOutput(ioutil.Discard)
}

type config struct {
	RedisURL    string
	PostgresDSN string
}

func getConfig() *config {
	c := &config{
		RedisURL: "redis://localhost:6379",
	}

	if v := os.Getenv("TEST_REDIS_URL"); v != "" {
		c.RedisURL = v
	}

	if v := os.Getenv("TEST_POSTGRES_DSN"); v != "" {
		c.PostgresDSN = v
	}

	return c
}

func mustFlushRedis(p *redis.Pool) {
	c := p.Get()
	defer c.Close()
	if _, err := c.Do("FLUSHALL"); err != nil {
		log.Fatal(err)
	}
}

func mustResetDB(db *sqlx.DB) {
	m, err := gomigrate.NewMigrator(db.DB, gomigrate.Postgres{}, "./migrations")
	if err != nil {
		log.Fatal(err)
	}
	if err := m.RollbackAll(); err != nil {
		log.Fatal(err)
	}
	if err := m.Migrate(); err != nil {
		log.Fatal(err)
	}
}

type testGatewayBackend struct {
	rxPacketChan chan RXPacket
	txPacketChan chan TXPacket
}

func (b *testGatewayBackend) Send(txPacket TXPacket) error {
	b.txPacketChan <- txPacket
	return nil
}

func (b *testGatewayBackend) RXPacketChan() chan RXPacket {
	return b.rxPacketChan
}

func (b *testGatewayBackend) Close() error {
	return nil
}

type testApplicationBackend struct {
	rxPacketsChan chan RXPackets
	err           error
}

func (b *testApplicationBackend) Send(devEUI, appEUI lorawan.EUI64, rxPackets RXPackets) error {
	b.rxPacketsChan <- rxPackets
	return b.err
}

func (b *testApplicationBackend) Close() error {
	return nil
}
