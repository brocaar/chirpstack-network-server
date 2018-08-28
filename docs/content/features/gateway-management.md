---
title: Gateway management
menu:
    main:
        parent: features
        weight: 1
---

# Gateway management

LoRa Server has support for managing gateways. Gateways can be created either
by enabling *create on stats* (see [gateway configuration]({{<ref "/install/config.md">}}))
or by using the [api]({{<ref "/integrate/api.md">}}).

## Gateway location

The (last known) location of the gateway will be stored in the database. When
the gateway is equipped with a GPS, its location will be automatically updated
after every stats update. Else, it can be manually set when creating or
updating the gateway.

## Gateway statistics

LoRa Server exposes the gateway statistics on a pre-configured aggregation
intervals (see [gateway configuration]({{<ref "/install/config.md">}})).
By default these intervals are configured to: minute, hour and day.

Note that LoRa Server does not implement any retention of the gateway stats.
In case you would like to keep

* the minute interval for one day
* the hour interval for one week
* the day interval for one year

you could use the following queries (e.g. as a cron):

```sql
delete from gateway_stats where "interval" = 'MINUTE' and "timestamp" < now() - interval '1 day';
delete from gateway_stats where "interval" = 'HOUR' and "timestamp" < now() - interval '1 week';
delete from gateway_stats where "interval" = 'DAY' and "timestamp" < now() - interval '1 year';
```


## Gateway re-configuration

If a [gateway-profile]({{<relref "gateway-profile.md">}}) is assigned
to the gateway, LoRa Server will push configuration updates to the gateway
to keep its channel-plan in sync with the configuration of the network.
This is triggered when LoRa Server receives gateway statistics (which also
contains the current configuration version of the gateway). If there is new
version of the configuration available, then it will be pushed by LoRa Server
to the gateway.

Note that this feature must also be configured in the
[LoRa Gateway Bridge configuration](/lora-gateway-bridge/install/config/).
