---
title: Source
menu:
    main:
        parent: community
        weight: 3
---

## LoRa Server source

Source-code can be found at [https://github.com/brocaar/loraserver](https://github.com/brocaar/loraserver).

### Building

#### With Docker

The easiest way to get started is by using the provided 
[docker-compose](https://docs.docker.com/compose/) environment. To start a bash
shell within the docker-compose environment, execute the following command from
the root of this project:

```bash
docker-compose run --rm loraserver bash
```

#### Without Docker

It is possible to build LoRa Server without Docker. However this requires
to install a couple of dependencies (depending your platform, there might be
pre-compiled packages available):

##### Go

Make sure you have [Go](https://golang.org/) installed (1.8+) and that the LoRa
App Server repository has been cloned into 
`$GOPATH/src/github.com/brocaar/loraserver`.

##### Go protocol buffer support

Install the C++ implementation of protocol buffers and Go support by following
the GO support for Protocol Buffers [installation instructions](https://github.com/golang/protobuf).

#### Example commands

A few example commands that you can run:

```bash
# install requirements
make requirements

# run the tests
make test

# compile
make build

# cross-compile for Linux ARM
GOOS=linux GOARCH=arm make build

# cross-compile for Windows AMD64
GOOS=windows BINEXT=.exe GOARCH=amd64 make build

# build the .tar.gz file for Linux ARM
GOOS=linux GOARCH=arm make package

# build the .tar.gz file for Windows AMD64
GOOS=windows BINEXT=.exe GOARCH=amd64 make package
```
