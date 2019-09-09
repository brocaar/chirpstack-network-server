package maccommand

import (
	"context"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRequestNewChannels(t *testing.T) {
	Convey("Given a set of tests", t, func() {
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

		for i, t := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
				out := RequestNewChannels(lorawan.EUI64{}, 3, t.CurrentChannels, t.WantedChannels)
				if t.ExpectedMACCommandBlock == nil {
					So(out, ShouldEqual, nil)
				} else {
					So(out, ShouldNotEqual, nil)
					So(*out, ShouldResemble, *t.ExpectedMACCommandBlock)
				}
			})
		}
	})
}

func TestHandleNewChannelAns(t *testing.T) {
	Convey("Given a set of tests", t, func() {
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
				},
			},
			{
				Name: "add new channels (nack)",
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					ExtraUplinkChannels:   map[int]band.Channel{},
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
				},
			},
			{
				Name: "modify existing channels",
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					ExtraUplinkChannels: map[int]band.Channel{
						3: band.Channel{Frequency: 868700000, MinDR: 3, MaxDR: 5},
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
				},
			},
		}

		for i, t := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
				ans, err := handleNewChannelAns(context.Background(), &t.DeviceSession, t.ReceivedMACCommandBlock, t.PendingMACCommandBlock)
				So(err, ShouldResemble, t.ExpectedError)
				So(ans, ShouldBeNil)
				So(t.DeviceSession, ShouldResemble, t.ExpectedDeviceSession)
			})
		}
	})
}
