package ack

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/internal/backend/controller"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

func TestDownlinkAck(t *testing.T) {
	assert := require.New(t)

	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))

	test.MustResetDB(storage.DB().DB)
	assert.NoError(storage.RedisClient().FlushAll().Err())

	asClient := test.NewApplicationClient()
	applicationserver.SetPool(test.NewApplicationServerPool(asClient))

	ncClient := test.NewNetworkControllerClient()
	controller.SetClient(ncClient)

	gwBackend := test.NewGatewayBackend()
	gateway.SetBackend(gwBackend)

	// create routing-profile
	rp := storage.RoutingProfile{
		ASID: "localhost:1234",
	}
	assert.NoError(storage.CreateRoutingProfile(context.Background(), storage.DB(), &rp))

	// dummy PHYPayload}
	var nwkSKey lorawan.AES128Key
	fPort := uint8(10)
	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataDown,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: lorawan.DevAddr{4, 3, 2, 1},
				FCtrl:   lorawan.FCtrl{},
				FCnt:    10,
				FOpts: []lorawan.Payload{
					&lorawan.MACCommand{
						CID: lorawan.DevStatusReq,
					},
				},
			},
			FPort:      &fPort,
			FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3}}},
		},
	}
	assert.NoError(phy.SetDownlinkDataMIC(lorawan.LoRaWAN1_0, 0, nwkSKey))
	phyB, err := phy.MarshalBinary()
	assert.NoError(err)

	// dummy txInfo
	txInfo1 := gw.DownlinkTXInfo{
		Frequency: 868100000,
	}
	txInfo2 := gw.DownlinkTXInfo{
		Frequency: 868300000,
	}

	tests := []struct {
		Name                          string
		DownlinkTxAck                 gw.DownlinkTXAck
		DownlinkFrameBefore           *storage.DownlinkFrame
		DownlinkFrameAfter            *storage.DownlinkFrame
		HandleDownlinkMetaDataRequest *nc.HandleDownlinkMetaDataRequest
		HandleTxAckRequest            *as.HandleTxAckRequest
		HandleErrorRequest            *as.HandleErrorRequest
		RetryDownlinkFrame            *gw.DownlinkFrame
	}{
		{
			Name: "Legacy ack",
			DownlinkTxAck: gw.DownlinkTXAck{
				Token: 1234,
				Error: "",
			},
			DownlinkFrameBefore: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo1,
							PhyPayload: phyB,
						},
					},
				},
			},
			HandleDownlinkMetaDataRequest: &nc.HandleDownlinkMetaDataRequest{
				DevEui:                      []byte{1, 2, 3, 4, 5, 6, 7, 8},
				TxInfo:                      &txInfo1,
				PhyPayloadByteCount:         17,
				MacCommandByteCount:         1,
				ApplicationPayloadByteCount: 3,
				MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
			},
			HandleTxAckRequest: &as.HandleTxAckRequest{
				DevEui: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				FCnt:   10,
				TxInfo: &txInfo1,
			},
		},
		{
			Name: "Legacy negative ack",
			DownlinkTxAck: gw.DownlinkTXAck{
				Token: 1234,
				Error: "TX_FREQ",
			},
			DownlinkFrameBefore: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo1,
							PhyPayload: phyB,
						},
					},
				},
			},
			HandleErrorRequest: &as.HandleErrorRequest{
				DevEui: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Type:   as.ErrorType_DATA_DOWN_GATEWAY,
				Error:  "TX_FREQ",
				FCnt:   10,
			},
		},
		{
			Name: "Legacy negative ack with retry",
			DownlinkTxAck: gw.DownlinkTXAck{
				Token: 1234,
				Error: "TX_FREQ",
			},
			DownlinkFrameBefore: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo1,
							PhyPayload: phyB,
						},
						{
							TxInfo:     &txInfo2,
							PhyPayload: phyB,
						},
					},
				},
			},
			DownlinkFrameAfter: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo2,
							PhyPayload: phyB,
						},
					},
				},
			},
			RetryDownlinkFrame: &gw.DownlinkFrame{
				Token: 1234,
				Items: []*gw.DownlinkFrameItem{
					{
						PhyPayload: phyB,
						TxInfo:     &txInfo2,
					},
				},
			},
		},
		{
			Name: "Single item ack",
			DownlinkTxAck: gw.DownlinkTXAck{
				Token: 1234,
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_OK,
					},
				},
			},
			DownlinkFrameBefore: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo1,
							PhyPayload: phyB,
						},
					},
				},
			},
			HandleDownlinkMetaDataRequest: &nc.HandleDownlinkMetaDataRequest{
				DevEui:                      []byte{1, 2, 3, 4, 5, 6, 7, 8},
				TxInfo:                      &txInfo1,
				PhyPayloadByteCount:         17,
				MacCommandByteCount:         1,
				ApplicationPayloadByteCount: 3,
				MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
			},
			HandleTxAckRequest: &as.HandleTxAckRequest{
				DevEui: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				FCnt:   10,
				TxInfo: &txInfo1,
			},
		},
		{
			Name: "Single item negative ack",
			DownlinkTxAck: gw.DownlinkTXAck{
				Token: 1234,
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_COLLISION_PACKET,
					},
				},
			},
			DownlinkFrameBefore: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo1,
							PhyPayload: phyB,
						},
					},
				},
			},
			HandleErrorRequest: &as.HandleErrorRequest{
				DevEui: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Type:   as.ErrorType_DATA_DOWN_GATEWAY,
				Error:  "COLLISION_PACKET",
				FCnt:   10,
			},
		},
		{
			Name: "Two items ack ok + ignored",
			DownlinkTxAck: gw.DownlinkTXAck{
				Token: 1234,
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_OK,
					},
					{
						Status: gw.TxAckStatus_IGNORED,
					},
				},
			},
			DownlinkFrameBefore: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo1,
							PhyPayload: phyB,
						},
						{
							TxInfo:     &txInfo2,
							PhyPayload: phyB,
						},
					},
				},
			},
			HandleDownlinkMetaDataRequest: &nc.HandleDownlinkMetaDataRequest{
				DevEui:                      []byte{1, 2, 3, 4, 5, 6, 7, 8},
				TxInfo:                      &txInfo1,
				PhyPayloadByteCount:         17,
				MacCommandByteCount:         1,
				ApplicationPayloadByteCount: 3,
				MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
			},
			HandleTxAckRequest: &as.HandleTxAckRequest{
				DevEui: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				FCnt:   10,
				TxInfo: &txInfo1,
			},
		},
		{
			Name: "Two items error + ok",
			DownlinkTxAck: gw.DownlinkTXAck{
				Token: 1234,
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_TX_FREQ,
					},
					{
						Status: gw.TxAckStatus_OK,
					},
				},
			},
			DownlinkFrameBefore: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo1,
							PhyPayload: phyB,
						},
						{
							TxInfo:     &txInfo2,
							PhyPayload: phyB,
						},
					},
				},
			},
			HandleDownlinkMetaDataRequest: &nc.HandleDownlinkMetaDataRequest{
				DevEui:                      []byte{1, 2, 3, 4, 5, 6, 7, 8},
				TxInfo:                      &txInfo2,
				PhyPayloadByteCount:         17,
				MacCommandByteCount:         1,
				ApplicationPayloadByteCount: 3,
				MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
			},
			HandleTxAckRequest: &as.HandleTxAckRequest{
				DevEui: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				FCnt:   10,
				TxInfo: &txInfo2,
			},
		},
		{
			Name: "Two items error + error",
			DownlinkTxAck: gw.DownlinkTXAck{
				Token: 1234,
				Items: []*gw.DownlinkTXAckItem{
					{
						Status: gw.TxAckStatus_TX_FREQ,
					},
					{
						Status: gw.TxAckStatus_COLLISION_PACKET,
					},
				},
			},
			DownlinkFrameBefore: &storage.DownlinkFrame{
				Token:            1234,
				DevEui:           []byte{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileId: rp.ID.Bytes(),
				FCnt:             uint32(10),
				NwkSEncKey:       nwkSKey[:],
				DownlinkFrame: &gw.DownlinkFrame{
					Token: 1234,
					Items: []*gw.DownlinkFrameItem{
						{
							TxInfo:     &txInfo1,
							PhyPayload: phyB,
						},
						{
							TxInfo:     &txInfo2,
							PhyPayload: phyB,
						},
					},
				},
			},
			HandleErrorRequest: &as.HandleErrorRequest{
				DevEui: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Type:   as.ErrorType_DATA_DOWN_GATEWAY,
				Error:  "COLLISION_PACKET",
				FCnt:   10,
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(storage.RedisClient().FlushAll().Err())

			// save downlink frame
			if tst.DownlinkFrameBefore != nil {
				assert.NoError(storage.SaveDownlinkFrame(context.Background(), *tst.DownlinkFrameBefore))
			}

			err := HandleDownlinkTXAck(context.Background(), tst.DownlinkTxAck)
			assert.NoError(err)

			// validate saved downlink frame
			if tst.DownlinkFrameAfter != nil {
				df, err := storage.GetDownlinkFrame(context.Background(), 1234)
				assert.NoError(err)
				assert.True(proto.Equal(tst.DownlinkFrameAfter, &df))
			}

			// validate controller meta-data call
			if tst.HandleDownlinkMetaDataRequest != nil {
				ncReq := <-ncClient.HandleDownlinkMetaDataChan
				assert.Equal(*tst.HandleDownlinkMetaDataRequest, ncReq)
			}

			// validate tx ack to as
			if tst.HandleTxAckRequest != nil {
				asReq := <-asClient.HandleTxAckChan
				assert.Equal(*tst.HandleTxAckRequest, asReq)
			}

			// validate error req
			if tst.HandleErrorRequest != nil {
				fmt.Println("AA")
				asReq := <-asClient.HandleErrorChan
				fmt.Println("BB")
				assert.Equal(*tst.HandleErrorRequest, asReq)
			}

			// validate downlink gateway frame
			if tst.RetryDownlinkFrame != nil {
				gwReq := <-gwBackend.TXPacketChan
				assert.True(proto.Equal(tst.RetryDownlinkFrame, &gwReq))
			}
		})
	}
}
