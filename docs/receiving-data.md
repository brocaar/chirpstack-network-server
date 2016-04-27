# Receiving data

To receive data from your node, you need to subscribe to its MQTT topic.
For debugging, you can use a (command-line) tool like ``mosquitto_sub``
which is part of the [Mosquitto](http://mosquitto.org/) MQTT broker.

Use ``+`` for a single-level wildcard, ``#`` for a multi-level wildcard.
Examples:

```bash
mosquitto_sub -t "application/0101010101010101/#" -v  # would display everything for the given application
mosquitto_sub -t "application/0101010101010101/node/+/rx" -v  # would display only the RX payloads for the given application
```

## application/[AppEUI]/node/[DevEUI]/rx

Topic for payloads received from your nodes. Example payload:

```json
{
    "devEUI": "0202020202020202",  // device EUI
    "fPort": 5,                    // FPort
    "gatewayCount": 3,             // number of gateways receiving this payload
	"rssi": -59,                   // signal strength
    "data": "..."                  // base64 encoded payload (decrypted)
}
```

## application/[AppEUI]/node/[DevEUI]/join

Topic for join notifications. Example payload:

```json
{
    "devAddr": "06682ea2",          // device address
    "DevEUI": "0202020202020202",   // device EUI
    "time": "2009-11-10T23:00:00Z"  // timestamp
}
```

## application/[AppEUI]/node/[DevEUI]/error

Topic for error notifications. An error might be raised when the downlink
payload size exceeded to max allowed payload size. Please see the LoRaWAN
specification for the max allowed payload size for your region. Example:

```json
{
    "reference": "abcd1234",    // the reference given when sending the downlink payload
    "message": "error message"  // the content of the error message
}
```
