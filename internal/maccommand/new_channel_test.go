package maccommand

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	nsband "github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

func TestNewChannel(t *testing.T) {
	assert := require.New(t)
	conf := test.GetConfig()
	assert.NoError(nsband.Setup(conf))

	t.Run("NewChannelReq", func(t *testing.T) {
		tests := []struct {
			Name                    string
			CurrentChannels         map[int]band.Channel
			WantedChannels          map[int]band.Channel
			ExpectedMACCommandBlock *storage.MACCommandBlock
		}{
			{
				Name: "adding new channel",
				CurrentChannels: map[int]band.Channel{
					3: band.Channel{Frequency: 868600000, MinDR: 3, MaxDR: 5},
					4: band.Channel{Frequency: 868700000, MinDR: 3, MaxDR: 5},
					5: band.Channel{Frequency: 868800000, MinDR: 3, MaxDR: 5},
				},
				WantedChannels: map[int]band.Channel{
					3: band.Channel{Frequency: 868600000, MinDR: 3, MaxDR: 5},
					4: band.Channel{Frequency: 868700000, MinDR: 3, MaxDR: 5},
					5: band.Channel{Frequency: 868800000, MinDR: 3, MaxDR: 5},
					6: band.Channel{Frequency: 868900000, MinDR: 3, MaxDR: 5},
					7: band.Channel{Frequency: 869000000, MinDR: 2, MaxDR: 5},
				},
				ExpectedMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.NewChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 6,
								Freq:    868900000,
								MinDR:   3,
								MaxDR:   5,
							},
						},
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 7,
								Freq:    869000000,
								MinDR:   2,
								MaxDR:   5,
							},
						},
					},
				},
			},
			{
				Name: "modifying channel",
				CurrentChannels: map[int]band.Channel{
					3: band.Channel{Frequency: 868600000, MinDR: 3, MaxDR: 5},
					4: band.Channel{Frequency: 868700000, MinDR: 3, MaxDR: 5},
					5: band.Channel{Frequency: 868800000, MinDR: 3, MaxDR: 5},
				},
				WantedChannels: map[int]band.Channel{
					3: band.Channel{Frequency: 868600000, MinDR: 3, MaxDR: 5},
					4: band.Channel{Frequency: 868650000, MinDR: 2, MaxDR: 4},
					5: band.Channel{Frequency: 868800000, MinDR: 3, MaxDR: 5},
				},
				ExpectedMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.NewChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 4,
								Freq:    868650000,
								MinDR:   2,
								MaxDR:   4,
							},
						},
					},
				},
			},
			{
				Name: "nothing to do",
				CurrentChannels: map[int]band.Channel{
					3: band.Channel{Frequency: 868600000, MinDR: 3, MaxDR: 5},
					4: band.Channel{Frequency: 868700000, MinDR: 3, MaxDR: 5},
					5: band.Channel{Frequency: 868800000, MinDR: 3, MaxDR: 5},
				},
				WantedChannels: map[int]band.Channel{
					3: band.Channel{Frequency: 868600000, MinDR: 3, MaxDR: 5},
					4: band.Channel{Frequency: 868700000, MinDR: 3, MaxDR: 5},
					5: band.Channel{Frequency: 868800000, MinDR: 3, MaxDR: 5},
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				resp := RequestNewChannels(lorawan.EUI64{}, 3, tst.CurrentChannels, tst.WantedChannels)
				if tst.ExpectedMACCommandBlock == nil {
					assert.Nil(resp)
				} else {
					assert.EqualValues(tst.ExpectedMACCommandBlock, resp)
				}
			})
		}
	})

	t.Run("handleNewChannelAns", func(t *testing.T) {
		tests := []struct {
			Name                    string
			DeviceSession           storage.DeviceSession
			ReceivedMACCommandBlock storage.MACCommandBlock
			PendingMACCommandBlock  *storage.MACCommandBlock
			ExpectedDeviceSession   storage.DeviceSession
			ExpectedError           error
		}{
			{
				Name: "add new channels (ack)",
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					ExtraUplinkChannels:   map[int]band.Channel{},
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.NewChannelAns: 1,
					},
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.NewChannelAns,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelAns,
							Payload: &lorawan.NewChannelAnsPayload{
								ChannelFrequencyOK: true,
								DataRateRangeOK:    true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.NewChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 3,
								Freq:    868600000,
								MinDR:   3,
								MaxDR:   5,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2, 3},
					ExtraUplinkChannels: map[int]band.Channel{
						3: band.Channel{Frequency: 868600000, MinDR: 3, MaxDR: 5},
					},
					MACCommandErrorCount: map[lorawan.CID]int{},
				},
			},
			{
				Name: "add new channels (nack)",
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					ExtraUplinkChannels:   map[int]band.Channel{},
					MACCommandErrorCount:  map[lorawan.CID]int{},
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.NewChannelAns,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelAns,
							Payload: &lorawan.NewChannelAnsPayload{
								ChannelFrequencyOK: false,
								DataRateRangeOK:    true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.NewChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 3,
								Freq:    868600000,
								MinDR:   3,
								MaxDR:   5,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					ExtraUplinkChannels:   map[int]band.Channel{},
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.NewChannelAns: 1,
					},
				},
			},
			{
				Name: "modify existing channels",
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					ExtraUplinkChannels: map[int]band.Channel{
						3: band.Channel{Frequency: 868700000, MinDR: 3, MaxDR: 5},
					},
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.NewChannelAns: 1,
					},
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.NewChannelAns,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelAns,
							Payload: &lorawan.NewChannelAnsPayload{
								ChannelFrequencyOK: true,
								DataRateRangeOK:    true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.NewChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 3,
								Freq:    868600000,
								MinDR:   3,
								MaxDR:   5,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2, 3},
					ExtraUplinkChannels: map[int]band.Channel{
						3: band.Channel{Frequency: 868600000, MinDR: 3, MaxDR: 5},
					},
					MACCommandErrorCount: map[lorawan.CID]int{},
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				assert := require.New(t)

				ans, err := handleNewChannelAns(context.Background(), &tst.DeviceSession, tst.ReceivedMACCommandBlock, tst.PendingMACCommandBlock)
				if tst.ExpectedError != nil {
					assert.Equal(tst.ExpectedError.Error(), err.Error())
					return
				}
				assert.NoError(err)
				assert.Nil(ans)
				assert.Equal(tst.ExpectedDeviceSession, tst.DeviceSession)
			})
		}
	})
}
