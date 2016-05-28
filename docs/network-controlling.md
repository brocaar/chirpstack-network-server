# Network controlling

LoRa Server does not implement a network-controller, but it provides
an api to integrate your own network-controller with LoRa Server.
Like with the application data, all communication is handled by using MQTT.

## Receiving data

### application/[AppEUI]/node/[DevEUI]/rxinfo

Topic on which RX related information is published for each received packet.
It combines the data from all gateways that received the same packet.
Example payload:

```json
{
	"devEUI": "0202020202020202",
	"adr": false,
	"fCnt": 1,
	"rxInfo": [{
		"mac": "1dee08d0b691d149",              // mac address of the gateway
		"time": "2016-05-01T10:50:54.973189Z",
		"timestamp": 1794488451,
		"frequency": 868100000,
		"channel": 0,
		"rfChain": 1,
		"crcStatus": 1,
		"codeRate": "4/5",
		"rssi": -54,
		"loRaSNR": 10.2,
		"size": 17,
		"dataRate": {
			"modulation": "LORA",
			"spreadFactor": 7,
			"bandwidth": 125
		}
	}]
}
```

### application/[AppEUI]/node/[DevEUI]/mac/rx

Topic for received MAC commands (from the nodes). Example payload:

```json
{
	"devEUI": "0202020202020202",  // device EUI
	"macCommand": "..."            // base64 encoded MAC command data
}
```

### application/[appEUI]/node/[DevEUI]/mac/error

Topic for error notifications. An error might be raised when the MAC comment
sent could not be processed. Example payload:

```json
{
    "reference": "abcd1234",    // the reference given when sending the MAC command
    "message": "error message"  // the content of the error message
}
```

## Sending data

### application/[AppEUI]/node/[DevEUI]/mac/tx

Topic for sending MAC commands to the node. Example payload:

```json
{
	"reference": "abcd1234",       // reference given by the network-controller
	"devEUI": "0202020202020202",  // the device to sent the MAC command to
	"macCommand": "..."            // base64 encoded MAC command data
}
```
