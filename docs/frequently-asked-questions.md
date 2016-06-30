# Frequently asked questions

## Does LoRa Server implement a queue for downlinks payloads?

Yes, all downlink payloads are stored in a queue (using Redis). Unconfirmed
downlink payloads are directly removed after sent. Confirmed downlink payloads
are removed from the queue after an `ACK` has been received by the node. When
there are multiple payloads, LoRa Server will set the `FRMPending` field to
`true` indicating that there is more data.

## Does LoRa Server support ADR?

Yes and no, LoRa Server itself doesn't support adaptive data-rate. However,
it exposes all the data so that you can implement your own network-controller.
See [network-controller](network-controller.md) for more information.

## Packets are not received / OTAA does not work

There are many things that can go wrong, and setting up a LoRaWAN
infrastructure can be frustrating when done the first time. First of all,
take a close look to the logs of LoRa Server and the [LoRa Gateway Bridge](http://docs.loraserver.io/lora-gateway-bridge/)
as it might give you a clue what is is going wrong. When that doesn't help,
take a look at the [packet_forwarder](https://github.com/Lora-net/packet_forwarder)
logs as it might give more information why packets are being dropped by the
gateway. The [LoRa Gateway Bridge FAQ](http://docs.loraserver.io/lora-gateway-bridge/frequently-asked-questions/)
has more information about debugging the packet_forwarder.

## I think I've found a bug

Please share bugs by creating a GitHub [issue](https://github.com/brocaar/loraserver/issues).
Be as descriptive as possible e.g.:

* LoRa Server and LoRa Gateway Bridge versions
* share logs of LoRa Server, LoRa Gateway Bridge
* what type of hardware are you using
* which region / frequencies
* how can the issue be replicated

## How can I contribute?

* Use the project and share your feedback :-)
* Help improving this documentation. All documentation is written in
  [Markdown](https://daringfireball.net/projects/markdown/syntax) format
  and can be found under the [docs](https://github.com/brocaar/loraserver/tree/master/docs)
  folder.
* If you would like to get involved in the (Go) development of LoRa Server please 
  share your ideas first by creating an [issue](https://github.com/brocaar/loraserver/issues)
  so your idea can be discussed. Since LoRa Server is under active development,
  it might be that your idea is already been worked on. 
