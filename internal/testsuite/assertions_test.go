package testsuite

import (
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

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

// AssertDownlinkFrame asserts the downlink frame.
func AssertDownlinkFrame(txInfo gw.DownlinkTXInfo, phy lorawan.PHYPayload) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		downlinkFrame := <-ts.GWBackend.TXPacketChan
		assert.NotEqual(0, downlinkFrame.Token)

		if !proto.Equal(&txInfo, downlinkFrame.TxInfo) {
			assert.Equal(txInfo, downlinkFrame.TxInfo)
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
			assert.NoError(downPHY.DecryptJoinAcceptPayload(ts.AppKey))
		}

		assert.Equal(phy, downPHY)
	}
}

// AssertNoDownlinkFrame asserts there is no downlink frame.
func AssertNoDownlinkFrame(assert *require.Assertions, ts *IntegrationTestSuite) {
	assert.Equal(0, len(ts.GWBackend.TXPacketChan))
}

// AssertMulticastQueueItems asserts the given multicast-queue items.
func AssertMulticastQueueItems(items []storage.MulticastQueueItem) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		mqi, err := storage.GetMulticastQueueItemsForMulticastGroup(ts.DB(), ts.MulticastGroup.ID)
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
		dqi, err := storage.GetDeviceQueueItemsForDevEUI(ts.DB(), ts.Device.DevEUI)
		assert.NoError(err)
		// avoid comparing nil vs empty slice
		if len(items) != len(dqi) {
			assert.Equal(items, dqi)
		}
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

// AssertDeviceSession asserts the given device-session.
func AssertDeviceSession(ds storage.DeviceSession) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		sess, err := storage.GetDeviceSession(ts.RedisPool(), ts.Device.DevEUI)
		assert.NoError(err)

		assert.NotEqual(lorawan.DevAddr{}, sess.DevAddr)
		sess.DevAddr = lorawan.DevAddr{}
		assert.Equal(ds, sess)
	}
}

// AssertDeviceActivation asserts the given device-activation.
func AssertDeviceActivation(da storage.DeviceActivation) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		act, err := storage.GetLastDeviceActivationForDevEUI(ts.DB(), ts.Device.DevEUI)
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
