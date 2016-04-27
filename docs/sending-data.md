# Sending data

Applications are able to send data to the nodes by using MQTT. All data sent
will be enqueued by LoRa Server until the next receive window with the node.
Note that max payload length restrictions apply, see the LoRaWAN specifications
for details about the restrictions for your region.

## application/[AppEUI]/node/[DevEUI]/tx

Example payload:

```json
{
    "reference": "abcd1234",       // reference given by the application, will be used on error
    "confirmed": true,             // whether the payload must be sent as confirmed data down or not
    "devEUI": "0202020202020202",  // the device to sent the data to
    "fPort": 10,                   // FPort to use
    "data": "...."                 // base64 encoded data (plaintext, will be encrypted by LoRa Server)
}

```
