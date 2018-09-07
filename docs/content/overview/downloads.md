---
title: Downloads
menu:
    main:
        parent: overview
        weight: 2
toc: false
---

# Downloads

## Precompiled binaries

| File name                                                                                                                                               | OS      | Arch  |
| ------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- | ----- |
| [loraserver_{{< version >}}_darwin_amd64.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_darwin_amd64.tar.gz)   | OS X    | amd64 |
| [loraserver_{{< version >}}_linux_386.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_linux_386.tar.gz)         | Linux   | 386   |
| [loraserver_{{< version >}}_linux_amd64.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_linux_amd64.tar.gz)     | Linux   | amd64 |
| [loraserver_{{< version >}}_linux_armv5.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_linux_armv5.tar.gz)     | Linux   | armv5 |
| [loraserver_{{< version >}}_linux_armv6.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_linux_armv6.tar.gz)     | Linux   | armv6 |
| [loraserver_{{< version >}}_linux_armv7.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_linux_armv7.tar.gz)     | Linux   | armv7 |
| [loraserver_{{< version >}}_linux_arm64.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_linux_arm64.tar.gz)     | Linux   | arm64 |
| [loraserver_{{< version >}}_windows_386.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_windows_386.tar.gz)     | Windows | 386   |
| [loraserver_{{< version >}}_windows_amd64.tar.gz](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_windows_amd64.tar.gz) | Windows | amd64 |

## Debian / Ubuntu packages

| File name                                                                                                                         | OS      | Arch  |
| ----------------------------------------------------------------------------------------------------------------------------------| ------- | ----- |
| [loraserver_{{< version >}}_i386.deb](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_i386.deb)   | Linux   | 386   |
| [loraserver_{{< version >}}_amd64.deb](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_amd64.deb) | Linux   | amd64 |
| [loraserver_{{< version >}}_armhf.deb](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_armhf.deb) | Linux   | arm   |
| [loraserver_{{< version >}}_arm64.deb](https://artifacts.loraserver.io/downloads/loraserver/loraserver_{{< version >}}_arm64.deb) | Linux   | arm64 |

## Debian / Ubuntu repository

As all packages are signed using a PGP key, you first need to import this key:

```bash
sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 1CE2AFD36DBCCA00
```

### Testing

The testing distribution contains the latest (test) releases.

```bash
sudo echo "deb https://artifacts.loraserver.io/packages/2.x/deb testing main" | sudo tee /etc/apt/sources.list.d/loraserver.list
sudo apt-get update
```

### Stable

The stable distribution contains releases that are considered stable.

```bash
sudo echo "deb https://artifacts.loraserver.io/packages/2.x/deb stable main" | sudo tee /etc/apt/sources.list.d/loraserver.list
sudo apt-get update
```

## Docker images

For Docker images, please refer to https://hub.docker.com/r/loraserver/loraserver/.
