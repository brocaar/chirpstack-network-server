package classb

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/chirpstack-network-server/internal/gps"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

func TestGetBeaconStartForTime(t *testing.T) {
	gpsEpochTime := time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

	Convey("When calling GetCurrentBeaconStart for GPS epoch time", t, func() {
		d := GetBeaconStartForTime(gpsEpochTime)

		Convey("Then the returned value == 0", func() {
			So(d, ShouldEqual, 0)
		})
	})

	Convey("When calling GetBeaconStartForTime for time.Now", t, func() {
		d := GetBeaconStartForTime(time.Now())

		Convey("Then the returned value > 0", func() {
			So(d, ShouldBeGreaterThan, 0)
		})

		Convey("Then the returned value is a multiple of 128 seconds", func() {
			So(d%beaconPeriod, ShouldEqual, 0)
		})

		Convey("Then the returned value is less than 128 seconds ago", func() {
			ts := time.Time(gps.NewFromTimeSinceGPSEpoch(d))
			So(ts.Before(time.Now()), ShouldBeTrue)

			So(time.Now().Sub(ts), ShouldBeLessThan, beaconPeriod)
		})
	})
}

func TestGetPingOffset(t *testing.T) {
	for k := uint(0); k < 8; k++ {
		var beacon time.Duration
		pingNb := 1 << k
		pingPeriod := pingPeriodBase / pingNb

		for test := 0; test < 100000; test++ {
			offset, err := GetPingOffset(beacon, lorawan.DevAddr{}, pingNb)
			if err != nil {
				t.Fatal(err)
			}

			if offset > pingPeriod-1 {
				t.Errorf("unexpected offset %d at pingNb %d test %d", offset, pingNb, test)
			}

			beacon += beaconPeriod
		}
	}
}

func TestGetNextPingSlotAfter(t *testing.T) {
	tests := []struct {
		After                    time.Duration
		DevAddr                  lorawan.DevAddr
		PingNb                   int
		ExpectedGPSEpochDuration string
		ExpectedError            error
	}{
		{
			After:                    0,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   1,
			ExpectedGPSEpochDuration: "1m14.3s",
		},
		{
			After:                    2 * time.Minute,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   1,
			ExpectedGPSEpochDuration: "3m5.62s",
		},
		{
			After:                    0,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   2,
			ExpectedGPSEpochDuration: "12.86s",
		},
		{
			After:                    13 * time.Second,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   2,
			ExpectedGPSEpochDuration: "1m14.3s",
		},
		{
			After:                    124 * time.Second,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   128,
			ExpectedGPSEpochDuration: "2m4.22s",
		},
	}

	for _, test := range tests {
		var expDuration time.Duration

		if test.ExpectedGPSEpochDuration != "" {
			var err error
			expDuration, err = time.ParseDuration(test.ExpectedGPSEpochDuration)
			if err != nil {
				t.Fatal(err)
			}
		}

		gpsEpochDuration, err := GetNextPingSlotAfter(test.After, test.DevAddr, test.PingNb)
		if err != test.ExpectedError {
			t.Errorf("expected error %s, got: %s", test.ExpectedError, err)
		}

		if gpsEpochDuration != expDuration {
			t.Errorf("expected gps epoch duration %s, got %s", test.ExpectedGPSEpochDuration, gpsEpochDuration)
		}
	}
}

func TestScheduleDeviceQueueToPingSlotsForDevEUI(t *testing.T) {
	conf := test.GetConfig()
	if err := storage.Setup(conf); err != nil {
		t.Fatal(err)
	}

	Convey("Given a clean database", t, func() {
		test.MustResetDB(storage.DB().DB)

		Convey("Given a device, device-session and three queue items", func() {
			sp := storage.ServiceProfile{}
			So(storage.CreateServiceProfile(context.Background(), storage.DB(), &sp), ShouldBeNil)

			dp := storage.DeviceProfile{
				ClassBTimeout: 30,
			}
			So(storage.CreateDeviceProfile(context.Background(), storage.DB(), &dp), ShouldBeNil)

			rp := storage.RoutingProfile{}
			So(storage.CreateRoutingProfile(context.Background(), storage.DB(), &rp), ShouldBeNil)

			d := storage.Device{
				DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				RoutingProfileID: rp.ID,
				ServiceProfileID: sp.ID,
				DeviceProfileID:  dp.ID,
			}
			So(storage.CreateDevice(context.Background(), storage.DB(), &d), ShouldBeNil)

			queueItems := []storage.DeviceQueueItem{
				{
					DevEUI:     d.DevEUI,
					FRMPayload: []byte{1, 2, 3, 4},
					FPort:      1,
					FCnt:       1,
				},
				{
					DevEUI:     d.DevEUI,
					FRMPayload: []byte{1, 2, 3, 4},
					FPort:      1,
					FCnt:       2,
				},
				{
					DevEUI:     d.DevEUI,
					FRMPayload: []byte{1, 2, 3, 4},
					FPort:      1,
					FCnt:       3,
				},
			}
			for i := range queueItems {
				So(storage.CreateDeviceQueueItem(context.Background(), storage.DB(), &queueItems[i]), ShouldBeNil)
			}

			ds := storage.DeviceSession{
				DevEUI:     d.DevEUI,
				DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
				PingSlotNb: 32,
			}

			Convey("When calling ScheduleDeviceQueueToPingSlotsForDevEUI", func() {
				timeSinceGPSEpochNow := gps.Time(time.Now()).TimeSinceGPSEpoch()
				So(ScheduleDeviceQueueToPingSlotsForDevEUI(context.Background(), storage.DB(), dp, ds), ShouldBeNil)

				Convey("Then each queue-item has a time since GPS epoch emit timestamp", func() {
					for i := range queueItems {
						qi, err := storage.GetDeviceQueueItem(context.Background(), storage.DB(), queueItems[i].ID)
						So(err, ShouldBeNil)

						So(qi.EmitAtTimeSinceGPSEpoch, ShouldNotBeNil)
						So(*qi.EmitAtTimeSinceGPSEpoch, ShouldBeGreaterThan, timeSinceGPSEpochNow)

						emitAt := time.Time(gps.NewFromTimeSinceGPSEpoch(*qi.EmitAtTimeSinceGPSEpoch))
						So(qi.TimeoutAfter.Equal(emitAt.Add(time.Second*time.Duration(dp.ClassBTimeout))), ShouldBeTrue)

						timeSinceGPSEpochNow = *qi.EmitAtTimeSinceGPSEpoch
					}
				})
			})
		})
	})
}
