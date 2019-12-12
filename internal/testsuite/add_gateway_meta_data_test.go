package testsuite

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/uplink"
	"github.com/brocaar/lorawan"
)

type AddGatewayMetaDataTestSuite struct {
	IntegrationTestSuite
}

func (ts *AddGatewayMetaDataTestSuite) SetupTest() {
	ts.IntegrationTestSuite.SetupTest()

	ts.CreateServiceProfile(storage.ServiceProfile{AddGWMetadata: true})
	ts.CreateGateway(storage.Gateway{GatewayID: lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}})
	ts.CreateDevice(storage.Device{DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}})
	ts.CreateDeviceSession(storage.DeviceSession{})
}

func (ts *AddGatewayMetaDataTestSuite) TestAddGatewayMetaData() {
	assert := require.New(ts.T())
	fpgaID := lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2}
	aesKey := lorawan.AES128Key{95, 234, 253, 54, 71, 53, 27, 235, 66, 63, 147, 206, 241, 74, 93, 219}

	now := time.Now().UTC().Round(time.Second)
	nowPB, err := ptypes.TimestampProto(now)
	assert.NoError(err)

	fineTime := now.Add(186118527 * time.Nanosecond)
	fineTimePB, err := ptypes.TimestampProto(fineTime)
	assert.NoError(err)

	tests := []struct {
		Name                   string
		FPGAID                 *lorawan.EUI64
		FineTimestampAESKey    *lorawan.AES128Key
		Location               storage.GPSPoint
		Altitude               float64
		EncryptedFineTimestamp *gw.EncryptedFineTimestamp

		ExpectedLocation      common.Location
		ExpectedFineTimestamp interface{}
	}{
		{
			Name: "location",
			Location: storage.GPSPoint{
				Latitude:  1.1234,
				Longitude: 2.1234,
			},
			Altitude: 3.1234,
			ExpectedLocation: common.Location{
				Latitude:  1.1234,
				Longitude: 2.1234,
				Altitude:  3.1234,
			},
		},
		{
			Name: "encrypted fine timestamp",
			Location: storage.GPSPoint{
				Latitude:  1.1234,
				Longitude: 2.1234,
			},
			Altitude: 3.1234,
			EncryptedFineTimestamp: &gw.EncryptedFineTimestamp{
				AesKeyIndex: 1,
				EncryptedNs: []byte{1, 2, 3, 4, 5},
			},
			FPGAID: &fpgaID,

			ExpectedLocation: common.Location{
				Latitude:  1.1234,
				Longitude: 2.1234,
				Altitude:  3.1234,
			},
			ExpectedFineTimestamp: &gw.EncryptedFineTimestamp{
				AesKeyIndex: 1,
				EncryptedNs: []byte{1, 2, 3, 4, 5},
				FpgaId:      fpgaID[:],
			},
		},
		{
			Name: "aes key decrypts fine timestamp",
			Location: storage.GPSPoint{
				Latitude:  1.1234,
				Longitude: 2.1234,
			},
			Altitude: 3.1234,
			EncryptedFineTimestamp: &gw.EncryptedFineTimestamp{
				AesKeyIndex: 1,
				EncryptedNs: []byte{239, 25, 15, 251, 170, 236, 252, 95, 216, 243, 142, 73, 104, 30, 105, 157},
			},
			FPGAID:              &fpgaID,
			FineTimestampAESKey: &aesKey,

			ExpectedLocation: common.Location{
				Latitude:  1.1234,
				Longitude: 2.1234,
				Altitude:  3.1234,
			},
			ExpectedFineTimestamp: &gw.PlainFineTimestamp{
				Time: fineTimePB,
			},
		},
	}

	for _, test := range tests {
		// flush clients and reload device-session as the frame-counter
		// increments on every test
		ts.FlushClients()
		ds, err := storage.GetDeviceSession(context.Background(), storage.RedisPool(), ts.DeviceSession.DevEUI)
		assert.NoError(err)
		ts.DeviceSession = &ds

		ts.T().Run(test.Name, func(t *testing.T) {
			assert := require.New(t)

			txInfo := gw.UplinkTXInfo{
				Frequency:  868300000,
				Modulation: common.Modulation_LORA,
				ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
					LoraModulationInfo: &gw.LoRaModulationInfo{
						SpreadingFactor: 10,
						Bandwidth:       125,
					},
				},
			}
			rxInfo := gw.UplinkRXInfo{
				GatewayId: ts.Gateway.GatewayID[:],
				Rssi:      -60,
				LoraSnr:   5.5,
				Time:      nowPB,
			}
			if test.EncryptedFineTimestamp != nil {
				rxInfo.FineTimestampType = gw.FineTimestampType_ENCRYPTED
				rxInfo.FineTimestamp = &gw.UplinkRXInfo_EncryptedFineTimestamp{
					EncryptedFineTimestamp: test.EncryptedFineTimestamp,
				}
			}

			ts.Gateway.Boards = []storage.GatewayBoard{
				{
					FPGAID:           test.FPGAID,
					FineTimestampKey: test.FineTimestampAESKey,
				},
			}

			ts.Gateway.Location = test.Location
			ts.Gateway.Altitude = test.Altitude
			assert.NoError(storage.UpdateGateway(context.Background(), storage.DB(), ts.Gateway))
			assert.NoError(storage.FlushGatewayCache(context.Background(), storage.RedisPool(), ts.Gateway.GatewayID))

			assert.Nil(uplink.HandleUplinkFrame(context.Background(), ts.GetUplinkFrameForFRMPayload(rxInfo, txInfo, lorawan.UnconfirmedDataUp, 10, []byte{1, 2, 3, 4})))

			ulDataReq := <-ts.ASClient.HandleDataUpChan
			assert.Len(ulDataReq.RxInfo, 1)

			rxItem := ulDataReq.RxInfo[0]
			rxItem.Location.XXX_sizecache = 0
			assert.Equal(&test.ExpectedLocation, rxItem.Location)

			switch rxItem.FineTimestampType {
			case gw.FineTimestampType_ENCRYPTED:
				tsInfo := rxItem.GetEncryptedFineTimestamp()
				tsInfo.XXX_sizecache = 0
				assert.Equal(test.ExpectedFineTimestamp, tsInfo)
			case gw.FineTimestampType_PLAIN:
				tsInfo := rxItem.GetPlainFineTimestamp()
				tsInfo.XXX_sizecache = 0
				tsInfo.Time.XXX_sizecache = 0
				assert.Equal(test.ExpectedFineTimestamp, tsInfo)
			}
		})
	}
}

func TestAddGatewayMetaData(t *testing.T) {
	suite.Run(t, new(AddGatewayMetaDataTestSuite))
}
