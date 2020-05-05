package testsuite

import (
	"context"
	"encoding/binary"
	"sort"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/geo"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/internal/downlink/ack"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

var lastToken uint32

// AssertFCntUp asserts the FCntUp.
func AssertFCntUp(fCnt uint32) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		assert.Equal(fCnt, ts.DeviceSession.FCntUp)
	}
}

// AssertNFCntDown asserts the NFCntDown.
func AssertNFCntDown(fCnt uint32) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		assert.Equal(fCnt, ts.DeviceSession.NFCntDown)
	}
}

// AssertAFCntDown asserts the AFCntDown.
func AssertAFCntDown(fCnt uint32) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		assert.Equal(fCnt, ts.DeviceSession.AFCntDown)
	}
}

// AssertMACCommandErrorCount asserts the mac-command error count.
func AssertMACCommandErrorCount(cid lorawan.CID, count int) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		assert.Equal(count, ts.DeviceSession.MACCommandErrorCount[cid])
	}
}

// AssertDownlinkFrame asserts the first downlink frame.
func AssertDownlinkFrame(gatewayID lorawan.EUI64, txInfo gw.DownlinkTXInfo, phy lorawan.PHYPayload) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		downlinkFrame := <-ts.GWBackend.TXPacketChan
		assert.NotEqual(0, downlinkFrame.Token)
		lastToken = downlinkFrame.Token

		assert.True(len(downlinkFrame.Items) != 0)
		assert.Equal(gatewayID[:], downlinkFrame.GatewayId)

		if !proto.Equal(&txInfo, downlinkFrame.Items[0].TxInfo) {
			assert.Equal(txInfo, downlinkFrame.Items[0].TxInfo)
		}

		switch phy.MHDR.MType {
		case lorawan.UnconfirmedDataDown, lorawan.ConfirmedDataDown:
			b, err := phy.MarshalBinary()
			assert.NoError(err)
			assert.NoError(phy.UnmarshalBinary(b))
			assert.NoError(phy.DecodeFOptsToMACCommands())
		}

		var downPHY lorawan.PHYPayload
		assert.NoError(downPHY.UnmarshalBinary(downlinkFrame.Items[0].PhyPayload))
		switch downPHY.MHDR.MType {
		case lorawan.UnconfirmedDataDown, lorawan.ConfirmedDataDown:
			assert.NoError(downPHY.DecodeFOptsToMACCommands())
		case lorawan.JoinAccept:
			assert.NoError(downPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))
		}

		assert.Equal(phy, downPHY)

		// ack the downlink transmission
		assert.NoError(ack.HandleDownlinkTXAck(context.Background(), gw.DownlinkTXAck{
			GatewayId: txInfo.GatewayId,
			Token:     downlinkFrame.Token,
		}))
	}
}

func AssertDownlinkFrameSaved(gatewayID lorawan.EUI64, devEUI lorawan.EUI64, mcGroupID uuid.UUID, txInfo gw.DownlinkTXInfo, phy lorawan.PHYPayload) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		df, err := storage.GetDownlinkFrame(context.Background(), uint16(lastToken))
		assert.NoError(err)

		assert.True(len(df.DownlinkFrame.Items) > 0, "empty downlink-frames")

		assert.Equal(gatewayID[:], df.DownlinkFrame.GatewayId)

		// if DevEUI is given, validate it
		var euiNil lorawan.EUI64
		if devEUI != euiNil {
			assert.Equal(devEUI[:], df.DevEui)
		}

		// if mc group id is given, validate it
		if mcGroupID != uuid.Nil {
			assert.Equal(mcGroupID[:], df.MulticastGroupId)
		}

		downlinkFrame := df.DownlinkFrame.Items[0]

		if !proto.Equal(&txInfo, downlinkFrame.TxInfo) {
			assert.Equal(txInfo, *downlinkFrame.TxInfo)
		}

		switch phy.MHDR.MType {
		case lorawan.UnconfirmedDataDown, lorawan.ConfirmedDataDown:
			b, err := phy.MarshalBinary()
			assert.NoError(err)
			assert.NoError(phy.UnmarshalBinary(b))
			assert.NoError(phy.DecodeFOptsToMACCommands())
		}

		var downPHY lorawan.PHYPayload
		assert.NoError(downPHY.UnmarshalBinary(downlinkFrame.PhyPayload))
		switch downPHY.MHDR.MType {
		case lorawan.UnconfirmedDataDown, lorawan.ConfirmedDataDown:
			assert.NoError(downPHY.DecodeFOptsToMACCommands())
		case lorawan.JoinAccept:
			assert.NoError(downPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))
		}

		assert.Equal(phy, downPHY)

		// pop the frame that we have been validating, so that we can validate the next one
		df.DownlinkFrame.Items = df.DownlinkFrame.Items[1:]
		assert.NoError(storage.SaveDownlinkFrame(context.Background(), df))
	}
}

// AssertNoDownlinkFrame asserts there is no downlink frame.
func AssertNoDownlinkFrame(assert *require.Assertions, ts *IntegrationTestSuite) {
	assert.Equal(0, len(ts.GWBackend.TXPacketChan))
}

func AssertNoDownlinkFrameSaved(assert *require.Assertions, ts *IntegrationTestSuite) {
	_, err := storage.GetDownlinkFrame(context.Background(), uint16(lastToken))
	assert.Equal(storage.ErrDoesNotExist, err)
}

// AssertMulticastQueueItems asserts the given multicast-queue items.
func AssertMulticastQueueItems(items []storage.MulticastQueueItem) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		mqi, err := storage.GetMulticastQueueItemsForMulticastGroup(context.Background(), storage.DB(), ts.MulticastGroup.ID)
		assert.NoError(err)
		// avoid comparing nil with empty slice
		if len(items) != len(mqi) {
			assert.Equal(items, mqi)
		}
	}
}

// AssertDeviceQueueItems asserts the device-queue items.
func AssertDeviceQueueItems(items []storage.DeviceQueueItem) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		dqi, err := storage.GetDeviceQueueItemsForDevEUI(context.Background(), storage.DB(), ts.Device.DevEUI)
		assert.NoError(err)
		// avoid comparing nil vs empty slice
		if len(items) != len(dqi) {
			assert.Equal(items, dqi)
		}
	}
}

// AssertDeviceMode asserts the current device class.
func AssertDeviceMode(mode storage.DeviceMode) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		d, err := storage.GetDevice(context.Background(), storage.DB(), ts.Device.DevEUI)
		assert.NoError(err)
		assert.Equal(mode, d.Mode)
	}
}

// AssertJSJoinReq asserts the given join-server JoinReq.
func AssertJSJoinReqPayload(pl backend.JoinReqPayload) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		req := <-ts.JSClient.JoinReqPayloadChan
		assert.NotEqual("", req.TransactionID)
		req.BasePayload.TransactionID = 0

		assert.NotEqual(lorawan.DevAddr{}, req.DevAddr)
		req.DevAddr = lorawan.DevAddr{}

		assert.Equal(req, pl)
	}
}

// AsssertJSRejoinReq asserts the given join-server RejoinReq.
func AssertJSRejoinReqPayload(pl backend.RejoinReqPayload) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		req := <-ts.JSClient.RejoinReqPayloadChan
		assert.NotEqual("", req.TransactionID)
		req.BasePayload.TransactionID = 0

		assert.NotEqual(lorawan.DevAddr{}, req.DevAddr)
		req.DevAddr = lorawan.DevAddr{}

		assert.Equal(pl, req)
	}
}

// AssertDeviceSession asserts the given device-session.
func AssertDeviceSession(ds storage.DeviceSession) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		sess, err := storage.GetDeviceSession(context.Background(), ts.Device.DevEUI)
		assert.NoError(err)

		assert.NotEqual(lorawan.DevAddr{}, sess.DevAddr)
		sess.DevAddr = lorawan.DevAddr{}

		if sess.PendingRejoinDeviceSession != nil {
			assert.NotEqual(lorawan.DevAddr{}, sess.PendingRejoinDeviceSession.DevAddr)
			sess.PendingRejoinDeviceSession.DevAddr = lorawan.DevAddr{}
		}

		assert.Equal(ds, sess)
	}
}

// AssertDeviceActivation asserts the given device-activation.
func AssertDeviceActivation(da storage.DeviceActivation) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		act, err := storage.GetLastDeviceActivationForDevEUI(context.Background(), storage.DB(), ts.Device.DevEUI)
		assert.NoError(err)

		assert.NotEqual(lorawan.DevAddr{}, act.DevAddr)
		act.DevAddr = lorawan.DevAddr{}

		assert.Equal(da, act)
	}
}

// AssertASHandleProprietaryUplinkRequest asserts the given proprietary uplink.
func AssertASHandleProprietaryUplinkRequest(req as.HandleProprietaryUplinkRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.HandleProprietaryUpChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertASHandleUplinkDataRequest asserts the given uplink request.
func AssertASHandleUplinkDataRequest(req as.HandleUplinkDataRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.HandleDataUpChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertASHandleDownlinkACKRequest asserts the given ack request.
func AssertASHandleDownlinkACKRequest(req as.HandleDownlinkACKRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.HandleDownlinkACKChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertASHandleTxAckRequest asserts the given tx ack request.
func AssertASHandleTxAckRequest(req as.HandleTxAckRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.HandleTxAckChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertASNoHandleTxAckRequest asserts that there is no tx ack request.
func AssertASNoHandleTxAckRequest() Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		time.Sleep(100 * time.Millisecond)
		select {
		case <-ts.ASClient.HandleTxAckChan:
			assert.Fail("unexpected tx ack request")
		default:
		}
	}
}

// AssertASHandleErrorRequest asserts the given error request.
func AssertASHandleErrorRequest(req as.HandleErrorRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.HandleErrorChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertASNoHandleErrorRequest asserts that there is no error request.
func AssertASNoHandleErrorRequest() Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		time.Sleep(100 * time.Millisecond)
		select {
		case <-ts.ASClient.HandleErrorChan:
			assert.Fail("unexpected error request")
		default:
		}
	}
}

// AssertNCHandleUplinkMACCommandRequest asserts the given mac-command request.
func AssertNCHandleUplinkMACCommandRequest(req nc.HandleUplinkMACCommandRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.NCClient.HandleDataUpMACCommandChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertNCHandleUplinkMetaDataRequest asserts the given uplink meta-data request.
func AssertNCHandleUplinkMetaDataRequest(req nc.HandleUplinkMetaDataRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.NCClient.HandleUplinkMetaDataChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertNCHandleDownlinkMetaDataRequest asserts the given downlink meta-data request.
func AssertNCHandleDownlinkMetaDataRequest(req nc.HandleDownlinkMetaDataRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.NCClient.HandleDownlinkMetaDataChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertTXPowerIndex asserts the given tx-power index.
func AssertTXPowerIndex(txPower int) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		assert.Equal(txPower, ts.DeviceSession.TXPowerIndex)
	}
}

// AssertNbTrans asserts the given NbTrans.
func AssertNbTrans(nbTrans int) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		assert.EqualValues(nbTrans, ts.DeviceSession.NbTrans)
	}
}

// AssertEnabledUplinkChannels asserts the enabled channels.
func AssertEnabledUplinkChannels(channels []int) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		assert.Equal(channels, ts.DeviceSession.EnabledUplinkChannels)
	}
}

// AssertASSetDeviceStatusRequests asserts the set device-status request.
func AssertASSetDeviceStatusRequest(req as.SetDeviceStatusRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.SetDeviceStatusChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertASSetDeviceLocationRequests asserts the set device-location request.
func AssertASSetDeviceLocationRequest(req as.SetDeviceLocationRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.SetDeviceLocationChan

		// we assume that we can sort on the first byte of each uplink_id
		sort.Slice(r.UplinkIds, func(i, j int) bool {
			return r.UplinkIds[i][0] < r.UplinkIds[j][0]
		})

		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertNoASSetDeviceLocationRequests asserts that there is no set device-location request.
func AssertNoASSetDeviceLocationRequest() Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		time.Sleep(100 * time.Millisecond)
		select {
		case <-ts.ASClient.SetDeviceLocationChan:
			assert.Fail("unexpected set device-location request")
		default:
		}
	}
}

// AssertResolveTDOARequest asserts the ResolveTDOARequest.
func AssertResolveTDOARequest(req geo.ResolveTDOARequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.GeoClient.ResolveTDOAChan
		sort.Sort(byGatewayID(r.FrameRxInfo.RxInfo))
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

// AssertResolveMultiFrameTDOARequest asserts the ResolveMultiFrameTDOARequest.
func AssertResolveMultiFrameTDOARequest(req geo.ResolveMultiFrameTDOARequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.GeoClient.ResolveMultiFrameTDOAChan
		for i := range r.FrameRxInfoSet {
			sort.Sort(byGatewayID(r.FrameRxInfoSet[i].RxInfo))
		}
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}

type byGatewayID []*gw.UplinkRXInfo

func (s byGatewayID) Len() int {
	return len(s)
}

func (s byGatewayID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byGatewayID) Less(i, j int) bool {
	ii := binary.BigEndian.Uint64(s[i].GatewayId)
	jj := binary.BigEndian.Uint64(s[j].GatewayId)
	return ii < jj
}
