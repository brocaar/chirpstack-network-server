package band

import (
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

var band loraband.Band

var maxLoRaDR int

// Setup sets up the band with the given configuration.
func Setup(c config.Config) error {
	dwellTime := lorawan.DwellTimeNoLimit
	if c.NetworkServer.Band.DownlinkDwellTime400ms {
		dwellTime = lorawan.DwellTime400ms
	}
	bandConfig, err := loraband.GetConfig(c.NetworkServer.Band.Name, c.NetworkServer.Band.RepeaterCompatible, dwellTime)
	if err != nil {
		return errors.Wrap(err, "get band config error")
	}
	for _, ec := range c.NetworkServer.NetworkSettings.ExtraChannels {
		if err := bandConfig.AddChannel(ec.Frequency, ec.MinDR, ec.MaxDR); err != nil {
			return errors.Wrap(err, "add channel error")
		}
	}
	band = bandConfig

	maxLoRaDR = 0
	enabledDRs := band.GetEnabledUplinkDataRates()
	for _, i := range enabledDRs {
		dr, err := band.GetDataRate(i)
		if err != nil {
			return errors.Wrap(err, "get max lora DR error")
		}

		if dr.Modulation == loraband.LoRaModulation && dr.Bandwidth == 125 {
			maxLoRaDR = i
		}
	}
	return nil
}

// Band returns the configured band.
func Band() loraband.Band {
	return band
}

func MaxLoRaDR() int {
	return maxLoRaDR
}