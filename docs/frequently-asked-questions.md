# Frequently asked questions

## Does LoRa Server implement a queue for downlinks payloads?

No, this component is now part of [LoRa App Server](http://docs.loraserver.io/lora-app-server/).

## Does LoRa Server support ADR?

Not yet, but this is on the roadmap.

## Packets are not received / OTAA does not work

There are many things that can go wrong, and setting up a LoRaWAN
infrastructure can be frustrating when done the first time. First of all,
take a close look to the logs of LoRa Server and the [LoRa Gateway Bridge](http://docs.loraserver.io/lora-gateway-bridge/)
as it might give you a clue what is is going wrong. When that doesn't help,
take a look at the [packet_forwarder](https://github.com/Lora-net/packet_forwarder)
logs as it might give more information why packets are being dropped by the
gateway. The [LoRa Gateway Bridge FAQ](http://docs.loraserver.io/lora-gateway-bridge/frequently-asked-questions/)
has more information about debugging the packet_forwarder.

## transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused

When you see the error `dial tcp 127.0.0.1:8001: getsockopt: connection refused`
in your logs, it means LoRa Server is unable to connect to the
application-server. See also [LoRa App Server](https://docs.loraserver.io/lora-app-server/).


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
