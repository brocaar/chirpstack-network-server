package loraserver

import (
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/migrations"
	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/rubenv/sql-migrate"
)

func init() {
	log.SetLevel(log.ErrorLevel)
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
	m := &migrate.AssetMigrationSource{
		Asset:    migrations.Asset,
		AssetDir: migrations.AssetDir,
		Dir:      "",
	}
	if _, err := migrate.Exec(db.DB, "postgres", m, migrate.Down); err != nil {
		log.Fatal(err)
	}
	if _, err := migrate.Exec(db.DB, "postgres", m, migrate.Up); err != nil {
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
	if b.rxPacketChan != nil {
		close(b.rxPacketChan)
	}
	return nil
}

type testApplicationBackend struct {
	txPayloadChan chan TXPayload
	rxPayloadChan chan RXPayload
	err           error
}

func (b *testApplicationBackend) Send(devEUI, appEUI lorawan.EUI64, payload RXPayload) error {
	b.rxPayloadChan <- payload
	return b.err
}

func (b *testApplicationBackend) Close() error {
	if b.txPayloadChan != nil {
		close(b.txPayloadChan)
	}
	return nil
}

func (b *testApplicationBackend) TXPayloadChan() chan TXPayload {
	return b.txPayloadChan
}
