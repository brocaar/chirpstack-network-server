package test

import "github.com/brocaar/chirpstack-api/go/v3/gw"

// GatewayBackend is a test gateway backend.
type GatewayBackend struct {
	rxPacketChan            chan gw.UplinkFrame
	TXPacketChan            chan gw.DownlinkFrame
	GatewayConfigPacketChan chan gw.GatewayConfiguration
	statsPacketChan         chan gw.GatewayStats
	downlinkTXAckChan       chan gw.DownlinkTXAck
}

// NewGatewayBackend returns a new GatewayBackend.
func NewGatewayBackend() *GatewayBackend {
	return &GatewayBackend{
		rxPacketChan:            make(chan gw.UplinkFrame, 100),
		TXPacketChan:            make(chan gw.DownlinkFrame, 100),
		GatewayConfigPacketChan: make(chan gw.GatewayConfiguration, 100),
		downlinkTXAckChan:       make(chan gw.DownlinkTXAck, 100),
	}
}

// SendTXPacket method.
func (b *GatewayBackend) SendTXPacket(txPacket gw.DownlinkFrame) error {
	b.TXPacketChan <- txPacket
	return nil
}

// SendGatewayConfigPacket method.
func (b *GatewayBackend) SendGatewayConfigPacket(config gw.GatewayConfiguration) error {
	b.GatewayConfigPacketChan <- config
	return nil
}

// RXPacketChan method.
func (b *GatewayBackend) RXPacketChan() chan gw.UplinkFrame {
	return b.rxPacketChan
}

// StatsPacketChan method.
func (b *GatewayBackend) StatsPacketChan() chan gw.GatewayStats {
	return b.statsPacketChan
}

// DownlinkTXAckChan method.
func (b *GatewayBackend) DownlinkTXAckChan() chan gw.DownlinkTXAck {
	return b.downlinkTXAckChan
}

// Close method.
func (b *GatewayBackend) Close() error {
	if b.rxPacketChan != nil {
		close(b.rxPacketChan)
	}
	return nil
}
