package api

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
)

type TestValidator struct {
	ctx context.Context
	mac lorawan.EUI64
}

func (v *TestValidator) GetMAC(ctx context.Context) (lorawan.EUI64, error) {
	v.ctx = ctx
	return v.mac, nil
}

func TestGatewayAPI(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean database and api instance", t, func() {
		db, err := common.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		test.MustResetDB(db)
		common.DB = db

		ctx := context.Background()

		validator := &TestValidator{}
		api := NewGatewayAPI(validator)

		Convey("Given a gateway", func() {
			g := gateway.Gateway{
				MAC:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				Name: "test-gateway",
			}
			validator.mac = g.MAC
			So(gateway.CreateGateway(db, &g), ShouldBeNil)

			Convey("Then calling GetConfiguration returns no items", func() {
				resp, err := api.GetConfiguration(ctx, &gw.GetConfigurationRequest{
					Mac: g.MAC[:],
				})
				So(err, ShouldBeNil)
				So(resp.Channels, ShouldHaveLength, 0)
			})

			Convey("Calling GetConfiguration for a different MAC than in the request context returns an error", func() {
				validator.mac = lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1}
				_, err := api.GetConfiguration(ctx, &gw.GetConfigurationRequest{
					Mac: g.MAC[:],
				})
				So(err, ShouldNotBeNil)
				So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			})

			Convey("Given channel-configuration with extra channels assigned to the gateway", func() {
				cc := gateway.ChannelConfiguration{
					Name:     "eu-conf",
					Channels: []int64{0, 1, 2},
					Band:     string(common.BandName),
				}
				So(gateway.CreateChannelConfiguration(db, &cc), ShouldBeNil)

				extraChannels := []gateway.ExtraChannel{
					{
						ChannelConfigurationID: cc.ID,
						Modulation:             gateway.ChannelModulationLoRa,
						Frequency:              867100000,
						BandWidth:              125,
						SpreadFactors:          []int64{12, 11, 10, 9, 8, 7},
					},
					{
						ChannelConfigurationID: cc.ID,
						Modulation:             gateway.ChannelModulationLoRa,
						Frequency:              868300000,
						BandWidth:              250,
						SpreadFactors:          []int64{7},
					},
					{
						ChannelConfigurationID: cc.ID,
						Modulation:             gateway.ChannelModulationFSK,
						Frequency:              868800000,
						BandWidth:              125,
						BitRate:                50000,
					},
				}

				for i := range extraChannels {
					So(gateway.CreateExtraChannel(db, &extraChannels[i]), ShouldBeNil)
				}

				g.ChannelConfigurationID = &cc.ID
				So(gateway.UpdateGateway(db, &g), ShouldBeNil)

				Convey("When calling GetConfiguration", func() {
					resp, err := api.GetConfiguration(ctx, &gw.GetConfigurationRequest{
						Mac: g.MAC[:],
					})
					So(err, ShouldBeNil)

					Convey("Then the UpdatedAt timestamp is around now", func() {
						ts, err := time.Parse(time.RFC3339Nano, resp.UpdatedAt)
						So(err, ShouldBeNil)

						So(time.Now().Sub(ts), ShouldBeBetween, time.Duration(0), time.Millisecond*50)
					})

					Convey("Then the expected channels are returned", func() {
						So(resp.Channels, ShouldHaveLength, 6)
						So(resp.Channels, ShouldResemble, []*gw.Channel{
							{
								Modulation:    gw.Modulation_LORA,
								Frequency:     868100000,
								Bandwidth:     125,
								SpreadFactors: []int32{12, 11, 10, 9, 8, 7},
							},
							{
								Modulation:    gw.Modulation_LORA,
								Frequency:     868300000,
								Bandwidth:     125,
								SpreadFactors: []int32{12, 11, 10, 9, 8, 7},
							},
							{
								Modulation:    gw.Modulation_LORA,
								Frequency:     868500000,
								Bandwidth:     125,
								SpreadFactors: []int32{12, 11, 10, 9, 8, 7},
							},
							{
								Modulation:    gw.Modulation_LORA,
								Frequency:     867100000,
								Bandwidth:     125,
								SpreadFactors: []int32{12, 11, 10, 9, 8, 7},
							},
							{
								Modulation:    gw.Modulation_LORA,
								Frequency:     868300000,
								Bandwidth:     250,
								SpreadFactors: []int32{7},
							},
							{
								Modulation: gw.Modulation_FSK,
								Frequency:  868800000,
								Bandwidth:  125,
								BitRate:    50000,
							},
						})
					})
				})
			})
		})
	})
}
