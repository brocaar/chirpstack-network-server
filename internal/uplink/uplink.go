package uplink

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/node"
	"github.com/brocaar/lorawan"
)

var flow = NewFlow().JoinRequest(
	setContextFromJoinRequestPHYPayload,
	logJoinRequestFramesCollected,
	getDeviceAndDeviceProfile,
	validateNonce,
	getRandomDevAddr,
	getJoinAcceptFromAS,
	logJoinRequestFrame,
	createNodeSession,
	createDeviceActivation,
	sendJoinAcceptDownlink,
).DataUp(
	setContextFromDataPHYPayload,
	getNodeSessionForDataUp,
	getServiceProfile,
	logDataFramesCollected,
	getApplicationServerClientForDataUp,
	decryptFRMPayloadMACCommands,
	sendRXInfoToNetworkController,
	handleFOptsMACCommands,
	handleFRMPayloadMACCommands,
	sendFRMPayloadToApplicationServer,
	handleChannelReconfiguration,
	handleADR,
	setLastRXInfoSet,
	syncUplinkFCnt,
	saveNodeSession,
	handleUplinkACK,
	handleDownlink,
).ProprietaryUp(
	setContextFromProprietaryPHYPayload,
	sendProprietaryPayloadToApplicationServer,
)

// Server represents a server listening for uplink packets.
type Server struct {
	wg sync.WaitGroup
}

// NewServer creates a new server.
func NewServer() *Server {
	return &Server{}
}

// Start starts the server.
func (s *Server) Start() error {
	go func() {
		s.wg.Add(1)
		defer s.wg.Done()
		HandleRXPackets(&s.wg)
	}()
	return nil
}

// Stop closes the gateway backend and waits for the server to complete the
// pending packets.
func (s *Server) Stop() error {
	if err := common.Gateway.Close(); err != nil {
		return fmt.Errorf("close gateway backend error: %s", err)
	}
	log.Info("waiting for pending actions to complete")
	s.wg.Wait()
	return nil
}

// HandleRXPackets consumes received packets by the gateway and handles them
// in a separate go-routine. Errors are logged.
func HandleRXPackets(wg *sync.WaitGroup) {
	for rxPacket := range common.Gateway.RXPacketChan() {
		go func(rxPacket gw.RXPacket) {
			wg.Add(1)
			defer wg.Done()
			if err := HandleRXPacket(rxPacket); err != nil {
				data, _ := rxPacket.PHYPayload.MarshalText()
				log.WithField("data_base64", string(data)).Errorf("processing rx packet error: %s", err)
			}
		}(rxPacket)
	}
}

// HandleRXPacket handles a single rxpacket.
func HandleRXPacket(rxPacket gw.RXPacket) error {
	return collectPackets(rxPacket)
}

func collectPackets(rxPacket gw.RXPacket) error {
	return collectAndCallOnce(common.RedisPool, rxPacket, func(rxPacket models.RXPacket) error {
		return flow.Run(rxPacket)
	})
}

func logUplink(db *sqlx.DB, devEUI lorawan.EUI64, rxPacket models.RXPacket) {
	if !common.LogNodeFrames {
		return
	}

	phyB, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		log.Errorf("marshal phypayload to binary error: %s", err)
		return
	}

	rxB, err := json.Marshal(rxPacket.RXInfoSet)
	if err != nil {
		log.Errorf("marshal rx-info set to json error: %s", err)
		return
	}

	fl := node.FrameLog{
		DevEUI:     devEUI,
		RXInfoSet:  &rxB,
		PHYPayload: phyB,
	}
	err = node.CreateFrameLog(db, &fl)
	if err != nil {
		log.Errorf("create frame-log error: %s", err)
	}
}
