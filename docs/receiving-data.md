# Receiving data

To receive data from a node, you need to subscribe to its MQTT topic. The format of this topic is `application/[AppEUI]/node/[DevEUI]/rx`, e.g. `application/0101010101010101/node/0202020202020202/rx`. All data on this topic is JSON encoded.

## Example payload

```json
{
    "devEUI": "0202020202020202",
    "fPort": 5,
    "gatewayCount": 3,
    "data": "..."
}
```

Note that the `data` key contains the decrypted payload in base64 encoding.
