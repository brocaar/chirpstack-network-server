package multicast

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/lorawan"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
)

type EnqueueQueueItemTestCase struct {
	suite.Suite
	test.DatabaseTestSuiteBase

	MulticastGroup storage.MulticastGroup
	Devices        []storage.Device
	Gateways       []storage.Gateway
}

func (ts *EnqueueQueueItemTestCase) SetupTest() {
	ts.DatabaseTestSuiteBase.SetupTest()
	assert := require.New(ts.T())

	var sp storage.ServiceProfile
	var rp storage.RoutingProfile

	assert.NoError(storage.CreateServiceProfile(ts.Tx(), &sp))
	assert.NoError(storage.CreateRoutingProfile(ts.Tx(), &rp))

	ts.MulticastGroup = storage.MulticastGroup{
		GroupType:        storage.MulticastGroupC,
		MCAddr:           lorawan.DevAddr{1, 2, 3, 4},
		MCNwkSKey:        lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		Frequency:        868100000,
		FCnt:             11,
		DR:               3,
		ServiceProfileID: sp.ID,
		RoutingProfileID: rp.ID,
	}
	assert.NoError(storage.CreateMulticastGroup(ts.Tx(), &ts.MulticastGroup))

	ts.Gateways = []storage.Gateway{
		{
			GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
		},
		{
			GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2},
		},
	}
	for i := range ts.Gateways {
		assert.NoError(storage.CreateGateway(ts.Tx(), &ts.Gateways[i]))
	}

	var dp storage.DeviceProfile

	assert.NoError(storage.CreateDeviceProfile(ts.Tx(), &dp))

	ts.Devices = []storage.Device{
		{
			DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
			ServiceProfileID: sp.ID,
			RoutingProfileID: rp.ID,
			DeviceProfileID:  dp.ID,
		},
		{
			DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
			ServiceProfileID: sp.ID,
			RoutingProfileID: rp.ID,
			DeviceProfileID:  dp.ID,
		},
	}
	for i := range ts.Devices {
		assert.NoError(storage.CreateDevice(ts.Tx(), &ts.Devices[i]))
		assert.NoError(storage.AddDeviceToMulticastGroup(ts.Tx(), ts.Devices[i].DevEUI, ts.MulticastGroup.ID))
		assert.NoError(storage.SaveDeviceGatewayRXInfoSet(ts.RedisPool(), storage.DeviceGatewayRXInfoSet{
			DevEUI: ts.Devices[i].DevEUI,
			DR:     3,
			Items: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: ts.Gateways[i].GatewayID,
					RSSI:      50,
					LoRaSNR:   5,
				},
			},
		}))
	}
}

func (ts *EnqueueQueueItemTestCase) TestInvalidFCnt() {
	assert := require.New(ts.T())
	assert.Equal(storage.MulticastGroupC, ts.MulticastGroup.GroupType)

	qi := storage.MulticastQueueItem{
		MulticastGroupID: ts.MulticastGroup.ID,
		FCnt:             10,
		FPort:            2,
		FRMPayload:       []byte{1, 2, 3, 4},
	}
	assert.Equal(ErrInvalidFCnt, EnqueueQueueItem(ts.RedisPool(), ts.Tx(), qi))
}

func (ts *EnqueueQueueItemTestCase) TestClassC() {
	assert := require.New(ts.T())
	assert.Equal(storage.MulticastGroupC, ts.MulticastGroup.GroupType)

	qi := storage.MulticastQueueItem{
		MulticastGroupID: ts.MulticastGroup.ID,
		FCnt:             11,
		FPort:            2,
		FRMPayload:       []byte{1, 2, 3, 4},
	}
	assert.NoError(EnqueueQueueItem(ts.RedisPool(), ts.Tx(), qi))

	items, err := storage.GetMulticastQueueItemsForMulticastGroup(ts.Tx(), ts.MulticastGroup.ID)
	assert.NoError(err)
	assert.Len(items, 2)

	assert.NotEqual(items[0].GatewayID, items[1].GatewayID)
	assert.Nil(items[0].EmitAtTimeSinceGPSEpoch)
	assert.Nil(items[1].EmitAtTimeSinceGPSEpoch)
	assert.EqualValues(math.Abs(float64(items[0].ScheduleAt.Sub(items[1].ScheduleAt))), config.ClassCDownlinkLockDuration)

	mg, err := storage.GetMulticastGroup(ts.Tx(), ts.MulticastGroup.ID, false)
	assert.NoError(err)
	assert.Equal(qi.FCnt+1, mg.FCnt)
}

func (ts *EnqueueQueueItemTestCase) TestClassB() {
	assert := require.New(ts.T())

	ts.MulticastGroup.PingSlotPeriod = 16
	ts.MulticastGroup.GroupType = storage.MulticastGroupB
	assert.NoError(storage.UpdateMulticastGroup(ts.Tx(), &ts.MulticastGroup))

	qi := storage.MulticastQueueItem{
		MulticastGroupID: ts.MulticastGroup.ID,
		FCnt:             11,
		FPort:            2,
		FRMPayload:       []byte{1, 2, 3, 4},
	}
	assert.NoError(EnqueueQueueItem(ts.RedisPool(), ts.Tx(), qi))

	items, err := storage.GetMulticastQueueItemsForMulticastGroup(ts.Tx(), ts.MulticastGroup.ID)
	assert.NoError(err)
	assert.Len(items, 2)
	assert.NotNil(items[0].EmitAtTimeSinceGPSEpoch)
	assert.NotNil(items[1].EmitAtTimeSinceGPSEpoch)
	assert.NotEqual(items[0].ScheduleAt, items[1].ScheduleAt)

	mg, err := storage.GetMulticastGroup(ts.Tx(), ts.MulticastGroup.ID, false)
	assert.NoError(err)
	assert.Equal(qi.FCnt+1, mg.FCnt)
}

func TestEnqueueQueueItem(t *testing.T) {
	suite.Run(t, new(EnqueueQueueItemTestCase))
}
