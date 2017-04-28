// Package band provides band specific defaults and configuration for
// downlink communication with end-nodes.
package band

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/brocaar/lorawan"
)

// Name defines the band-name type.
type Name string

// Available ISM bands.
const (
	AS_923     Name = "AS_923"
	AU_915_928 Name = "AU_915_928"
	CN_470_510 Name = "CN_470_510"
	CN_779_787 Name = "CN_779_787"
	EU_433     Name = "EU_433"
	EU_863_870 Name = "EU_863_870"
	KR_920_923 Name = "KR_920_923"
	RU_864_869 Name = "RU_864_869"
	US_902_928 Name = "US_902_928"
)

// Modulation defines the modulation type.
type Modulation string

// Possible modulation types.
const (
	LoRaModulation Modulation = "LORA"
	FSKModulation  Modulation = "FSK"
)

// DataRate defines a data rate
type DataRate struct {
	Modulation   Modulation `json:"modulation"`
	SpreadFactor int        `json:"spreadFactor,omitempty"` // used for LoRa
	Bandwidth    int        `json:"bandwidth,omitempty"`    // in kHz, used for LoRa
	BitRate      int        `json:"bitRate,omitempty"`      // bits per second, used for FSK
}

// MaxPayloadSize defines the max payload size
type MaxPayloadSize struct {
	M int // The maximum MACPayload size length
	N int // The maximum application payload length in the absence of the optional FOpt control field
}

// Channel defines the channel structure
type Channel struct {
	Frequency      int   // frequency in Hz
	DataRates      []int // each int mapping to an index in DataRateConfiguration
	userConfigured bool  // user-configured channel
	deactivated    bool  // used to deactivate on or multiple channels (e.g. for US ISM band)
}

// Band defines an region specific ISM band implementation for LoRa.
type Band struct {
	// dwellTime defines if dwell time limitation should be taken into account
	dwellTime lorawan.DwellTime

	// rx1DataRate defines the RX1 data-rate given the uplink data-rate
	// and a RX1DROffset value. These values are retrievable by using
	// the GetRX1DataRate method.
	rx1DataRate [][]int

	// DefaultTXPower defines the default radiated transmit output power
	DefaultTXPower int

	// ImplementsCFlist defines if the band implements the optional channel
	// frequency list.
	ImplementsCFlist bool

	// RX2Frequency defines the fixed frequency for the RX2 receive window
	RX2Frequency int

	// RX2DataRate defines the fixed data-rate for the RX2 receive window
	RX2DataRate int

	// MaxFcntGap defines the MAC_FCNT_GAP default value.
	MaxFCntGap uint32

	// ADRACKLimit defines the ADR_ACK_LIMIT default value.
	ADRACKLimit int

	// ADRACKDelay defines the ADR_ACK_DELAY default value.
	ADRACKDelay int

	// ReceiveDelay1 defines the RECEIVE_DELAY1 default value.
	ReceiveDelay1 time.Duration

	// ReceiveDelay2 defines the RECEIVE_DELAY2 default value.
	ReceiveDelay2 time.Duration

	// JoinAcceptDelay1 defines the JOIN_ACCEPT_DELAY1 default value.
	JoinAcceptDelay1 time.Duration

	// JoinAcceptDelay2 defines the JOIN_ACCEPT_DELAY2 default value.
	JoinAcceptDelay2 time.Duration

	// ACKTimeoutMin defines the ACK_TIMEOUT min. default value.
	ACKTimeoutMin time.Duration

	// ACKTimeoutMax defines the ACK_TIMEOUT max. default value.
	ACKTimeoutMax time.Duration

	// DataRates defines the available data rates.
	DataRates []DataRate

	// MaxPayloadSize defines the maximum payload size, per data-rate.
	MaxPayloadSize []MaxPayloadSize

	// TXPower defines the TX power configuration.
	TXPower []int

	// UplinkChannels defines the list of (default) configured uplink channels.
	UplinkChannels []Channel

	// DownlinkChannels defines the list of (default) configured downlink
	// channels.
	DownlinkChannels []Channel

	// getRX1ChannelFunc implements a function which returns the RX1 channel
	// based on the uplink / TX channel.
	getRX1ChannelFunc func(txChannel int) int

	// getRX1FrequencyFunc implements a function which returns the RX1 frequency
	// given the uplink frequency.
	getRX1FrequencyFunc func(band *Band, txFrequency int) (int, error)

	// getRX1DataRateFunc implements a function which returns the RX1 data-rate
	// given the uplink data-rate and data-rate offset.
	getRX1DataRateFunc func(band *Band, uplinkDR, rx1DROffset int) (int, error)

	// getLinkADRReqPayloadsForEnabledChannelsFunc implements a band-specific
	// function which returns the LinkADRReqPayload items needed to activate
	// the used channels on the node.
	// In case this is set GetLinkADRReqPayloadsForEnabledChannels will
	// use both the "naive" algorithm and the algorithm used in this function.
	// The smallest result is then returned.
	// In case this function is left blank, only the "naive" algorithm is used.
	getLinkADRReqPayloadsForEnabledChannelsFunc func(band *Band, nodeChannels []int) []lorawan.LinkADRReqPayload

	// getEnabledChannelsForLinkADRReqPayloadsFunc implements a band-specific
	// function (taking ChMaskCntl values with band-specific meaning into
	// account).
	getEnabledChannelsForLinkADRReqPayloadsFunc func(band *Band, nodeChannels []int, pls []lorawan.LinkADRReqPayload) ([]int, error)
}

// GetRX1Channel returns the channel to use for RX1 given the channel used
// for uplink.
func (b *Band) GetRX1Channel(txChannel int) int {
	return b.getRX1ChannelFunc(txChannel)
}

// GetRX1Frequency returns the frequency to use for RX1 given the uplink
// frequency.
func (b *Band) GetRX1Frequency(txFrequency int) (int, error) {
	return b.getRX1FrequencyFunc(b, txFrequency)
}

// GetRX1DataRate returns the RX1 data-rate given the uplink data-rate and
// RX1 data-rate offset.
func (b *Band) GetRX1DataRate(uplinkDR, rx1DROffset int) (int, error) {
	// use the lookup table when no function has been defined
	if b.getRX1DataRateFunc == nil {
		if uplinkDR > len(b.rx1DataRate)-1 {
			return 0, errors.New("lorawan/band: invalid data-rate")
		}
		if rx1DROffset > len(b.rx1DataRate[uplinkDR])-1 {
			return 0, errors.New("lorawan/band: invalid data-rate offset")
		}
		return b.rx1DataRate[uplinkDR][rx1DROffset], nil
	}
	return b.getRX1DataRateFunc(b, uplinkDR, rx1DROffset)
}

// GetUplinkChannelNumber returns the channel number given a frequency.
func (b *Band) GetUplinkChannelNumber(frequency int) (int, error) {
	for chanNum, channel := range b.UplinkChannels {
		if frequency == channel.Frequency {
			return chanNum, nil
		}
	}

	return 0, fmt.Errorf("lorawan/band: unknown channel for frequency: %d", frequency)
}

// GetDataRate returns the index of the given DataRate.
func (b *Band) GetDataRate(dr DataRate) (int, error) {
	for i, d := range b.DataRates {
		if d == dr {
			return i, nil
		}
	}
	return 0, errors.New("lorawan/band: the given data-rate does not exist")
}

// AddChannel adds an extra (user-configured) channel to the channels.
// The DataRates wil be set to DR 0-5.
// Note: this is only allowed when the band supports a CFList.
func (b *Band) AddChannel(freq int) error {
	if !b.ImplementsCFlist {
		return errors.New("lorawan/band: band does not implement CFList")
	}

	c := Channel{
		Frequency:      freq,
		DataRates:      []int{0, 1, 2, 3, 4, 5},
		userConfigured: true,
		deactivated:    freq == 0,
	}

	b.UplinkChannels = append(b.UplinkChannels, c)
	b.DownlinkChannels = append(b.DownlinkChannels, c)

	return nil
}

// GetCFList returns the CFList used for OTAA activation, or returns nil if
// the band does not implement the CFList or when there are no extra channels.
// Note that this only returns the first 5 extra channels.
func (b *Band) GetCFList() *lorawan.CFList {
	if !b.ImplementsCFlist {
		return nil
	}

	var cFList lorawan.CFList
	var i int
	for _, c := range b.UplinkChannels {
		if c.userConfigured && i < len(cFList) {
			cFList[i] = uint32(c.Frequency)
			i++
		}
	}

	if cFList[0] == 0 {
		return nil
	}
	return &cFList
}

// DisableUplinkChannel disables the given uplink channel.
func (b *Band) DisableUplinkChannel(i int) error {
	if i > len(b.UplinkChannels)-1 {
		return ErrChannelDoesNotExist
	}

	b.UplinkChannels[i].deactivated = true
	return nil
}

// EnableUplinkChannel enables the given uplink channel.
func (b *Band) EnableUplinkChannel(i int) error {
	if i > len(b.UplinkChannels)-1 {
		return ErrChannelDoesNotExist
	}

	b.UplinkChannels[i].deactivated = false
	return nil
}

// GetUplinkChannels returns all available uplink channels.
func (b *Band) GetUplinkChannels() []int {
	var out []int
	for i := range b.UplinkChannels {
		out = append(out, i)
	}
	return out
}

// GetEnabledUplinkChannels returns the enabled uplink channels.
func (b *Band) GetEnabledUplinkChannels() []int {
	var out []int
	for i, c := range b.UplinkChannels {
		if !c.deactivated {
			out = append(out, i)
		}
	}
	return out
}

// GetDisabledUplinkChannels returns the disabled uplink channels.
func (b *Band) GetDisabledUplinkChannels() []int {
	var out []int
	for i, c := range b.UplinkChannels {
		if c.deactivated {
			out = append(out, i)
		}
	}
	return out
}

// GetLinkADRReqPayloadsForEnabledChannels returns the LinkADRReqPayloads to
// reconfigure the node to the current active channels. Note that in case of
// activation, user-defined channels (e.g. CFList) will be ignored as it
// is unknown if the node is aware about these extra frequencies.
func (b *Band) GetLinkADRReqPayloadsForEnabledChannels(nodeChannels []int) []lorawan.LinkADRReqPayload {
	enabledChannels := b.GetEnabledUplinkChannels()
	var enabledChannelsNoCFList []int

	for _, c := range enabledChannels {
		if !b.UplinkChannels[c].userConfigured {
			enabledChannelsNoCFList = append(enabledChannelsNoCFList, c)
		}
	}

	diff := intSliceDiff(nodeChannels, enabledChannels)

	// nothing to do
	if len(diff) == 0 || len(intSliceDiff(nodeChannels, enabledChannelsNoCFList)) == 0 {
		return nil
	}

	// make sure we're dealing with a sorted slice
	sort.Ints(diff)

	var payloads []lorawan.LinkADRReqPayload
	chMaskCntl := -1

	// loop over the channel blocks that contain different channels
	// note that each payload holds 16 channels and that the chMaskCntl
	// defines the block
	for _, c := range diff {
		if c/16 != chMaskCntl {
			chMaskCntl = c / 16
			pl := lorawan.LinkADRReqPayload{
				Redundancy: lorawan.Redundancy{
					ChMaskCntl: uint8(chMaskCntl),
				},
			}

			// set enabled channels in this block to active
			// note that we don't enable user defined channels (CFList) as
			// we have no knowledge if the nodes has been provisioned with
			// these frequencies
			for _, ec := range enabledChannels {
				if (!b.UplinkChannels[ec].userConfigured || channelIsActive(nodeChannels, ec)) && ec >= chMaskCntl*16 && ec < (chMaskCntl+1)*16 {
					pl.ChMask[ec%16] = true
				}
			}

			payloads = append(payloads, pl)
		}
	}

	// some bands contain band specific logic regarding turning on / off
	// channels that might require less commands (using band specific
	// ChMaskCntl values)
	if b.getLinkADRReqPayloadsForEnabledChannelsFunc != nil {
		payloadsB := b.getLinkADRReqPayloadsForEnabledChannelsFunc(b, nodeChannels)
		if len(payloadsB) < len(payloads) {
			return payloadsB
		}
	}

	return payloads
}

// GetEnabledChannelsForLinkADRReqPaylaods returns the enabled after which the
// given LinkADRReqPayloads have been applied to the given node channels.
func (b *Band) GetEnabledChannelsForLinkADRReqPayloads(nodeChannels []int, pls []lorawan.LinkADRReqPayload) ([]int, error) {
	// for some bands some the ChMaskCntl values have special meanings
	if b.getEnabledChannelsForLinkADRReqPayloadsFunc != nil {
		return b.getEnabledChannelsForLinkADRReqPayloadsFunc(b, nodeChannels, pls)
	}

	chMask := make([]bool, len(b.UplinkChannels))
	for _, c := range nodeChannels {
		// make sure that we don't exceed the chMask length. in case we exceed
		// we ignore the channel as it might have been removed from the network
		if c < len(chMask) {
			chMask[c] = true
		}
	}

	for _, pl := range pls {
		for i, enabled := range pl.ChMask {
			if int(pl.Redundancy.ChMaskCntl*16)+i >= len(chMask) && !enabled {
				continue
			}

			if int(pl.Redundancy.ChMaskCntl*16)+i >= len(chMask) {
				return nil, ErrChannelDoesNotExist
			}

			chMask[int(pl.Redundancy.ChMaskCntl*16)+i] = enabled
		}
	}

	// turn the chMask into a slice of enabled channel numbers
	var out []int
	for i, enabled := range chMask {
		if enabled {
			out = append(out, i)
		}
	}

	return out, nil
}

// intSliceDiff returns all items of x that are not in y and all items of
// y that are not in x.
func intSliceDiff(x, y []int) []int {
	var out []int

	for _, cX := range x {
		found := false
		for _, cY := range y {
			if cX == cY {
				found = true
				break
			}
		}
		if !found {
			out = append(out, cX)
		}
	}

	for _, cY := range y {
		found := false
		for _, cX := range x {
			if cY == cX {
				found = true
				break
			}
		}
		if !found {
			out = append(out, cY)
		}
	}

	return out
}

func channelIsActive(channels []int, i int) bool {
	for _, c := range channels {
		if i == c {
			return true
		}
	}
	return false
}

// GetConfig returns the band configuration for the given band.
// Please refer to the LoRaWAN specification for more details about the effect
// of the repeater and dwell time arguments.
func GetConfig(name Name, repeaterCompatible bool, dt lorawan.DwellTime) (Band, error) {
	switch name {
	case AS_923:
		return newAS923Band(repeaterCompatible, dt)
	case AU_915_928:
		return newAU915Band(repeaterCompatible)
	case CN_470_510:
		return newCN470Band()
	case CN_779_787:
		return newCN779Band(repeaterCompatible)
	case EU_433:
		return newEU433Band(repeaterCompatible)
	case EU_863_870:
		return newEU863Band(repeaterCompatible)
	case KR_920_923:
		return newKR920Band()
	case RU_864_869:
		return newRU864Band(repeaterCompatible)
	case US_902_928:
		return newUS902Band(repeaterCompatible)
	default:
		return Band{}, fmt.Errorf("lorawan/band: band %s is undefined", name)
	}
}
