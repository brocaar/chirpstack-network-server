package testsuite

import (
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/storage"
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

// AssertDownlinkFrame asserts the downlink frame.
func AssertDownlinkFrame(txInfo gw.DownlinkTXInfo, phy lorawan.PHYPayload) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		downlinkFrame := <-ts.GWBackend.TXPacketChan
		assert.NotEqual(0, downlinkFrame.Token)
		lastToken = downlinkFrame.Token

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
			assert.NoError(downPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))
		}

		assert.Equal(phy, downPHY)
	}
}

func AssertDownlinkFrameSaved(devEUI lorawan.EUI64, txInfo gw.DownlinkTXInfo, phy lorawan.PHYPayload) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		eui, downlinkFrame, err := storage.PopDownlinkFrame(storage.RedisPool(), lastToken)
		assert.NoError(err)

		assert.Equal(devEUI, eui)

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
			assert.NoError(downPHY.DecryptJoinAcceptPayload(ts.JoinAcceptKey))
		}

		assert.Equal(phy, downPHY)
	}
}

// AssertNoDownlinkFrame asserts there is no downlink frame.
func AssertNoDownlinkFrame(assert *require.Assertions, ts *IntegrationTestSuite) {
	assert.Equal(0, len(ts.GWBackend.TXPacketChan))
}

func AssertNoDownlinkFrameSaved(assert *require.Assertions, ts *IntegrationTestSuite) {
	_, _, err := storage.PopDownlinkFrame(storage.RedisPool(), lastToken)
	assert.Equal(storage.ErrDoesNotExist, err)
}

// AssertMulticastQueueItems asserts the given multicast-queue items.
func AssertMulticastQueueItems(items []storage.MulticastQueueItem) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		mqi, err := storage.GetMulticastQueueItemsForMulticastGroup(storage.DB(), ts.MulticastGroup.ID)
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
		dqi, err := storage.GetDeviceQueueItemsForDevEUI(storage.DB(), ts.Device.DevEUI)
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
		d, err := storage.GetDevice(storage.DB(), ts.Device.DevEUI)
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
		sess, err := storage.GetDeviceSession(storage.RedisPool(), ts.Device.DevEUI)
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
		act, err := storage.GetLastDeviceActivationForDevEUI(storage.DB(), ts.Device.DevEUI)
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

// AssertASHandleErrorRequest asserts the given error request.
func AssertASHandleErrorRequest(req as.HandleErrorRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.HandleErrorChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
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

func AssertASSetDeviceStatusRequest(req as.SetDeviceStatusRequest) Assertion {
	return func(assert *require.Assertions, ts *IntegrationTestSuite) {
		r := <-ts.ASClient.SetDeviceStatusChan
		if !proto.Equal(&r, &req) {
			assert.Equal(req, r)
		}
	}
}
