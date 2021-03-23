package testsuite

import (
	"context"
	"testing"
	"time"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type DownlinkTXAckTestSuite struct {
	IntegrationTestSuite
}

func (ts *DownlinkTXAckTestSuite) getPHYPayload(mType lorawan.MType, fPort *uint8, fOpts []lorawan.Payload, frmPayload []lorawan.Payload) []byte {
	// we are just interested in the MType to differentiate between confirmed and unconfirmed
	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: mType,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				FOpts: fOpts,
			},
			FPort:      fPort,
			FRMPayload: frmPayload,
		},
	}

	b, _ := phy.MarshalBinary()
	return b
}

func (ts *DownlinkTXAckTestSuite) TestDownlinkTXAck() {
	ts.CreateMulticastGroup(storage.MulticastGroup{
		GroupType: storage.MulticastGroupC,
	})

	ts.CreateDevice(storage.Device{
		DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	})

	ts.CreateMulticastGroup(storage.MulticastGroup{
		MCAddr:    lorawan.DevAddr{4, 3, 2, 1},
		FCnt:      30,
		GroupType: storage.MulticastGroupC,
		DR:        0,
		Frequency: 868100000,
	})

	ts.CreateGateway(storage.Gateway{
		GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
	})

	ds10 := storage.DeviceSession{
		DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
		DevEUI:           ts.Device.DevEUI,
		DeviceProfileID:  ts.DeviceProfile.ID,
		ServiceProfileID: ts.ServiceProfile.ID,
		RoutingProfileID: ts.RoutingProfile.ID,
		MACVersion:       "1.0.3",
		NFCntDown:        10,
	}

	ds11 := storage.DeviceSession{
		DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		DeviceProfileID:  ts.DeviceProfile.ID,
		ServiceProfileID: ts.ServiceProfile.ID,
		RoutingProfileID: ts.RoutingProfile.ID,
		MACVersion:       "1.1.0",
		NFCntDown:        10,
		AFCntDown:        20,
	}

	fPort2 := uint8(2)
	classBTimeout := time.Now().Add(10 * time.Second)

	ts.T().Run("LW10 - Class-A", func(t *testing.T) {
		tests := []DownlinkTXAckTest{
			{
				Name:          "ack for unconfirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
					AssertNFCntDown(11),
					AssertASHandleTxAckRequest(as.HandleTxAckRequest{
						DevEui:    ts.Device.DevEUI[:],
						FCnt:      10,
						GatewayId: ts.Gateway.GatewayID[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
					}),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						DevEui: ts.Device.DevEUI[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         16,
						MacCommandByteCount:         0,
						ApplicationPayloadByteCount: 3,
						MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name:          "nack for unconfirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_FREQ,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{
						{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10},
					}),
					AssertNFCntDown(10),
					AssertASHandleErrorRequest(as.HandleErrorRequest{
						DevEui: ts.Device.DevEUI[:],
						Type:   as.ErrorType_DATA_DOWN_GATEWAY,
						Error:  "TX_FREQ",
					}),
					AssertASNoHandleTxAckRequest(),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
			{
				Name:          "ack for confirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.ConfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true},
				},
				Assert: []Assertion{
					AssertDeviceQueueItemsFunc(func(assert *require.Assertions, items []storage.DeviceQueueItem) {
						assert.Len(items, 1)
						assert.True(items[0].IsPending)
					}),
					AssertNFCntDown(11),
					AssertASHandleTxAckRequest(as.HandleTxAckRequest{
						DevEui:    ts.Device.DevEUI[:],
						FCnt:      10,
						GatewayId: ts.Gateway.GatewayID[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
					}),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						DevEui: ts.Device.DevEUI[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         16,
						MacCommandByteCount:         0,
						ApplicationPayloadByteCount: 3,
						MessageType:                 nc.MType_CONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name:          "nack for confirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_FREQ,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.ConfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true},
				},
				Assert: []Assertion{
					AssertDeviceQueueItemsFunc(func(assert *require.Assertions, items []storage.DeviceQueueItem) {
						assert.Len(items, 1)
						assert.False(items[0].IsPending)
					}),
					AssertNFCntDown(10),
					AssertASNoHandleTxAckRequest(),
					AssertASHandleErrorRequest(as.HandleErrorRequest{
						DevEui: ts.Device.DevEUI[:],
						Type:   as.ErrorType_DATA_DOWN_GATEWAY,
						Error:  "TX_FREQ",
					}),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
			{
				Name:          "ack for unconfirmed mac frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, nil, []lorawan.Payload{
									&lorawan.MACCommand{CID: lorawan.DevStatusReq},
								}, nil),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				Assert: []Assertion{
					AssertNFCntDown(11),
					AssertASNoHandleErrorRequest(),
					AssertASNoHandleTxAckRequest(),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						DevEui: ts.Device.DevEUI[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         13,
						MacCommandByteCount:         1,
						ApplicationPayloadByteCount: 0,
						MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name:          "nack for unconfirmed mac frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_FREQ,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token: 123,
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, nil, nil, nil),
								TxInfo:     &gw.DownlinkTXInfo{},
							},
						},
					},
				},
				Assert: []Assertion{
					AssertNFCntDown(10),
					AssertASNoHandleErrorRequest(), // no error as this is mac-layer only
					AssertASNoHandleTxAckRequest(),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				ts.AssertDownlinkTXAckTest(t, tst)
			})
		}
	})

	ts.T().Run("LW11 - Class-A", func(t *testing.T) {
		tests := []DownlinkTXAckTest{
			{
				Name:          "ack for unconfirmed app frame",
				DeviceSession: ds11,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds11.DevAddr, DevEUI: ds11.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 20},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
					AssertNFCntDown(10),
					AssertAFCntDown(21),
					AssertASHandleTxAckRequest(as.HandleTxAckRequest{
						DevEui:    ts.Device.DevEUI[:],
						FCnt:      20,
						GatewayId: ts.Gateway.GatewayID[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
					}),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						DevEui: ts.Device.DevEUI[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         16,
						MacCommandByteCount:         0,
						ApplicationPayloadByteCount: 3,
						MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name:          "nack for unconfirmed app frame",
				DeviceSession: ds11,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_FREQ,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token: 123,
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds11.DevAddr, DevEUI: ds11.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 20},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{
						{DevAddr: ds11.DevAddr, DevEUI: ds11.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 20},
					}),
					AssertNFCntDown(10),
					AssertAFCntDown(20),
					AssertASHandleErrorRequest(as.HandleErrorRequest{
						DevEui: ts.Device.DevEUI[:],
						Type:   as.ErrorType_DATA_DOWN_GATEWAY,
						Error:  "TX_FREQ",
					}),
					AssertASNoHandleTxAckRequest(),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				ts.AssertDownlinkTXAckTest(t, tst)
			})
		}
	})

	ts.T().Run("LW10 - Class-B", func(t *testing.T) {
		tests := []DownlinkTXAckTest{
			{
				Name:          "ack for unconfirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, TimeoutAfter: &classBTimeout},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
					AssertNFCntDown(11),
					AssertASHandleTxAckRequest(as.HandleTxAckRequest{
						DevEui:    ts.Device.DevEUI[:],
						FCnt:      10,
						GatewayId: ts.Gateway.GatewayID[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
					}),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						DevEui: ts.Device.DevEUI[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         16,
						MacCommandByteCount:         0,
						ApplicationPayloadByteCount: 3,
						MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name:          "nack for unconfirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_POWER,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token: 123,
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, TimeoutAfter: &classBTimeout},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{
						{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, TimeoutAfter: &classBTimeout},
					}),
					AssertNFCntDown(10),
					AssertASNoHandleTxAckRequest(),
					AssertASHandleErrorRequest(as.HandleErrorRequest{
						DevEui: ts.Device.DevEUI[:],
						Type:   as.ErrorType_DATA_DOWN_GATEWAY,
						Error:  "TX_POWER",
					}),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
			{
				Name:          "ack for confirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.ConfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true, TimeoutAfter: &classBTimeout},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{
						{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true, TimeoutAfter: &classBTimeout, IsPending: true},
					}),
					AssertNFCntDown(11),
					AssertASHandleTxAckRequest(as.HandleTxAckRequest{
						DevEui:    ts.Device.DevEUI[:],
						FCnt:      10,
						GatewayId: ts.Gateway.GatewayID[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
					}),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						DevEui: ts.Device.DevEUI[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         16,
						MacCommandByteCount:         0,
						ApplicationPayloadByteCount: 3,
						MessageType:                 nc.MType_CONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name:          "nack for confirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_POWER,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token: 123,
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.ConfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true, TimeoutAfter: &classBTimeout},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{
						{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true, TimeoutAfter: &classBTimeout, IsPending: true},
					}),
					AssertNFCntDown(10),
					AssertASHandleErrorRequest(as.HandleErrorRequest{
						DevEui: ts.Device.DevEUI[:],
						Type:   as.ErrorType_DATA_DOWN_GATEWAY,
						Error:  "TX_POWER",
					}),
					AssertASNoHandleTxAckRequest(),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				ts.AssertDownlinkTXAckTest(t, tst)
			})
		}
	})

	ts.T().Run("LW10 - Class-C", func(t *testing.T) {
		assert := require.New(t)
		ts.DeviceProfile.SupportsClassC = true
		ts.DeviceProfile.ClassCTimeout = 10
		assert.NoError(storage.UpdateDeviceProfile(context.Background(), storage.DB(), ts.DeviceProfile))

		tests := []DownlinkTXAckTest{
			{
				Name:          "ack for unconfirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{}),
					AssertNFCntDown(11),
					AssertASHandleTxAckRequest(as.HandleTxAckRequest{
						DevEui:    ts.Device.DevEUI[:],
						FCnt:      10,
						GatewayId: ts.Gateway.GatewayID[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
					}),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						DevEui: ts.Device.DevEUI[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         16,
						MacCommandByteCount:         0,
						ApplicationPayloadByteCount: 3,
						MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name:          "nack for unconfirmed LoRaWAN 1.0 Class-C app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_POWER,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{
						{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10},
					}),
					AssertNFCntDown(10),
					AssertASNoHandleTxAckRequest(),
					AssertASHandleErrorRequest(as.HandleErrorRequest{
						DevEui: ts.Device.DevEUI[:],
						Type:   as.ErrorType_DATA_DOWN_GATEWAY,
						Error:  "TX_POWER",
					}),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
			{
				Name:          "ack for confirmed LoRaWAN 1.0 Class-C app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.ConfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true},
				},
				Assert: []Assertion{
					AssertDeviceQueueItemsFunc(func(assert *require.Assertions, items []storage.DeviceQueueItem) {
						assert.Len(items, 1)
						assert.True(items[0].IsPending)
						assert.True(items[0].TimeoutAfter.After(time.Now()))
					}),
					AssertNFCntDown(11),
					AssertASHandleTxAckRequest(as.HandleTxAckRequest{
						DevEui:    ts.Device.DevEUI[:],
						FCnt:      10,
						GatewayId: ts.Gateway.GatewayID[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
					}),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						DevEui: ts.Device.DevEUI[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         16,
						MacCommandByteCount:         0,
						ApplicationPayloadByteCount: 3,
						MessageType:                 nc.MType_CONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name:          "nack for confirmed app frame",
				DeviceSession: ds10,
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_POWER,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					DevEui:           ts.Device.DevEUI[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.ConfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				DeviceQueueItems: []storage.DeviceQueueItem{
					{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true},
				},
				Assert: []Assertion{
					AssertDeviceQueueItems([]storage.DeviceQueueItem{
						{DevAddr: ds10.DevAddr, DevEUI: ds10.DevEUI, FRMPayload: []byte{1, 2, 3}, FPort: 2, FCnt: 10, Confirmed: true},
					}),
					AssertNFCntDown(10),
					AssertASNoHandleTxAckRequest(),
					AssertASHandleErrorRequest(as.HandleErrorRequest{
						DevEui: ts.Device.DevEUI[:],
						Type:   as.ErrorType_DATA_DOWN_GATEWAY,
						Error:  "TX_POWER",
					}),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				ts.AssertDownlinkTXAckTest(t, tst)
			})
		}
	})

	ts.T().Run("LW10 - Multicast", func(t *testing.T) {
		tests := []DownlinkTXAckTest{
			{
				Name: "ack for multicast frame",
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_OK,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					MulticastGroupId: ts.MulticastGroup.ID[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				MulticastQueueItems: []storage.MulticastQueueItem{
					{ScheduleAt: time.Now(), MulticastGroupID: ts.MulticastGroup.ID, GatewayID: ts.Gateway.GatewayID, FCnt: 30, FPort: 2},
				},
				Assert: []Assertion{
					// The multicast downlink fCnt is incremented by the scheduler, as a
					// downlink can be emitted by multiple gateways.
					AssertMulticastGroupFCntDown(30),
					AssertMulticastQueueItems([]storage.MulticastQueueItem{}),
					AssertASNoHandleTxAckRequest(),
					AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
						MulticastGroupId: ts.MulticastGroup.ID[:],
						TxInfo: &gw.DownlinkTXInfo{
							Frequency: 868100000,
						},
						PhyPayloadByteCount:         16,
						MacCommandByteCount:         0,
						ApplicationPayloadByteCount: 3,
						MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
						GatewayId:                   ts.Gateway.GatewayID[:],
					}),
				},
			},
			{
				Name: "nack for multicast frame",
				DownlinkTXAck: &gw.DownlinkTXAck{
					Token: 123,
					Items: []*gw.DownlinkTXAckItem{
						{
							Status: gw.TxAckStatus_TX_FREQ,
						},
					},
				},
				DownlinkFrame: &storage.DownlinkFrame{
					Token:            123,
					MulticastGroupId: ts.MulticastGroup.ID[:],
					RoutingProfileId: ts.RoutingProfile.ID[:],
					DownlinkFrame: &gw.DownlinkFrame{
						Token:     123,
						GatewayId: ts.Gateway.GatewayID[:],
						Items: []*gw.DownlinkFrameItem{
							{
								PhyPayload: ts.getPHYPayload(lorawan.UnconfirmedDataDown, &fPort2, nil, []lorawan.Payload{
									&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
								}),
								TxInfo: &gw.DownlinkTXInfo{
									Frequency: 868100000,
								},
							},
						},
					},
				},
				MulticastQueueItems: []storage.MulticastQueueItem{
					{ScheduleAt: time.Now(), MulticastGroupID: ts.MulticastGroup.ID, GatewayID: ts.Gateway.GatewayID, FCnt: 30, FPort: 2},
				},
				Assert: []Assertion{
					AssertMulticastGroupFCntDown(30),
					/*
						AssertMulticastQueueItems([]storage.MulticastQueueItem{
							{ScheduleAt: time.Now(), MulticastGroupID: ts.MulticastGroup.ID, GatewayID: ts.Gateway.GatewayID, FCnt: 30, FPort: 2},
						}),
					*/
					// For now the item is removed from the queue.
					AssertMulticastQueueItems([]storage.MulticastQueueItem{}),
					AssertASNoHandleErrorRequest(),
					AssertASNoHandleTxAckRequest(),
					AssertNCNoHandleDownlinkMetaDataRequest(),
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				ts.AssertDownlinkTXAckTest(t, tst)
			})
		}
	})
}

func TestDownlinkTXAck(t *testing.T) {
	suite.Run(t, new(DownlinkTXAckTestSuite))
}
