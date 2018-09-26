package testsuite

import (
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
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

		b, err := phy.MarshalBinary()
		assert.NoError(err)
		assert.NoError(phy.UnmarshalBinary(b))
		assert.NoError(phy.DecodeFOptsToMACCommands())

		var downPHY lorawan.PHYPayload
		assert.NoError(downPHY.UnmarshalBinary(downlinkFrame.PhyPayload))
		assert.NoError(downPHY.DecodeFOptsToMACCommands())
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
		mqi, err := storage.GetMulticastQueueItemsForMulticastGroup(config.C.PostgreSQL.DB, ts.MulticastGroup.ID)
		assert.NoError(err)
		// avoid comparing nil with empty slice
		if len(items) != len(mqi) {
			assert.Equal(items, mqi)
		}
	}
}
