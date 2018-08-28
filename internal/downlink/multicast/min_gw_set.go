package multicast

import (
	"encoding/binary"
	"math"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// GetMinimumGatewaySet returns the minimum set of gateways to cover all
// devices.
func GetMinimumGatewaySet(rxInfoSets []storage.DeviceGatewayRXInfoSet) ([]lorawan.EUI64, error) {
	g := simple.NewWeightedUndirectedGraph(0, math.Inf(1))

	gwSet := getGatewaySet(rxInfoSets)

	// connect all gateways
	// W -999 is used so that the mst algorithm will remove the edge between
	// the gateway and a device first, over removing an edge between two
	// gateways.
	for i, gatewayID := range gwSet {
		if i == 0 {
			continue
		}

		g.SetWeightedEdge(simple.WeightedEdge{
			F: simple.Node(eui64Int64(gwSet[0])),
			T: simple.Node(eui64Int64(gatewayID)),
			W: -999,
		})
	}

	// connect all devices to the gateways
	addDeviceEdges(g, rxInfoSets)

	dst := simple.NewWeightedUndirectedGraph(0, math.Inf(1))
	path.Kruskal(dst, g)

	outMap := make(map[lorawan.EUI64]struct{})

	for _, e := range dst.Edges() {
		fromEUI := int64ToEUI64(e.From().ID())
		toEUI := int64ToEUI64(e.To().ID())

		fromIsGW := gwInGWSet(fromEUI, gwSet)
		toIsGW := gwInGWSet(toEUI, gwSet)

		// skip gateway to gateway edges
		if !(fromIsGW && toIsGW) {
			if fromIsGW {
				outMap[fromEUI] = struct{}{}
			}

			if toIsGW {
				outMap[toEUI] = struct{}{}
			}
		}
	}

	var outSlice []lorawan.EUI64
	for k := range outMap {
		outSlice = append(outSlice, k)
	}

	return outSlice, nil
}

func getGatewaySet(rxInfoSets []storage.DeviceGatewayRXInfoSet) []lorawan.EUI64 {
	gwSet := make(map[lorawan.EUI64]struct{})
	for _, rxInfoSet := range rxInfoSets {
		for _, rxInfo := range rxInfoSet.Items {
			gwSet[rxInfo.GatewayID] = struct{}{}
		}
	}

	var out []lorawan.EUI64
	for k := range gwSet {
		out = append(out, k)
	}

	return out
}

func eui64Int64(eui lorawan.EUI64) int64 {
	return int64(binary.BigEndian.Uint64(eui[:]))
}

func int64ToEUI64(i int64) lorawan.EUI64 {
	var eui lorawan.EUI64
	binary.BigEndian.PutUint64(eui[:], uint64(i))
	return eui
}

func gwInGWSet(gatewayID lorawan.EUI64, gwSet []lorawan.EUI64) bool {
	var found bool
	for _, id := range gwSet {
		if gatewayID == id {
			found = true
		}
	}

	return found
}

func addDeviceEdges(g *simple.WeightedUndirectedGraph, rxInfoSets []storage.DeviceGatewayRXInfoSet) {
	for _, rxInfo := range rxInfoSets {
		dr, err := config.C.NetworkServer.Band.Band.GetDataRate(rxInfo.DR)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"dr": dr,
			}).Error("invalid data-data")
		}

		reqSNR, ok := config.SpreadFactorToRequiredSNRTable[dr.SpreadFactor]
		if ok {
			reqSNR += config.C.NetworkServer.NetworkSettings.InstallationMargin
		}

		var hasReqSNR bool

		for _, item := range rxInfo.Items {
			if item.LoRaSNR >= reqSNR {
				hasReqSNR = true
			}
		}

		for _, item := range rxInfo.Items {
			// ignore items that do not have the min. required SNR value,
			// knowning that we have items that do meet the min. req SNR.
			if item.LoRaSNR < reqSNR && hasReqSNR {
				continue
			}

			g.SetWeightedEdge(deviceGatewayEdge{
				gatewayID: item.GatewayID,
				devEUI:    rxInfo.DevEUI,
				graph:     g,
			})
		}
	}
}

type deviceGatewayEdge struct {
	gatewayID lorawan.EUI64
	devEUI    lorawan.EUI64
	graph     *simple.WeightedUndirectedGraph
}

// From implements graph.Edge.
func (e deviceGatewayEdge) From() graph.Node {
	return simple.Node(eui64Int64(e.devEUI))
}

// From implements graph.Edge.
func (e deviceGatewayEdge) To() graph.Node {
	return simple.Node(eui64Int64(e.gatewayID))
}

// Weight implements graph.WeightedEdge.
// The returned weight is equal to 1 / number of devices covered by the
// gateway.
func (e deviceGatewayEdge) Weight() float64 {
	weight := float64(1)

	gwNodes := e.graph.From(eui64Int64(e.gatewayID))
	if len(gwNodes) != 0 {
		weight = weight / float64(len(gwNodes))
	}

	return weight
}
