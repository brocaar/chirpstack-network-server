# Sending data

Applications are able to send data back to the node by sending a payload over MQTT. The MQTT topic which the application needs to use is `application/[AppEUI]/node/[DevEUI]/tx`, e.g. `application/0101010101010101/node/0202020202020202/tx`.

Note that data will only be sent once a receive window is open (which is initiated by the node).

LoRa Server will enqueue all received payloads (and in the case there are multiple payloads to send, it will tell the node that there are more frames pending). When sending a confirmed payload, LoRa Server will keep the payload in the queue until the node has sent back an ack.

## Example

```json
{
    "confirmed": true,
    "devEUI": "0202020202020202",
    "fPort": 10,
    "data": "...."
}
```

Note that the payload needs to be JSON encoded. The `data` key must be the plaintext data in base64 encoding. LoRa Server will take care of the encryption.
