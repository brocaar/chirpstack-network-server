---
title: Gateway management
menu:
    main:
        parent: use
        weight: 2
---

## Gateway management

LoRa Server has support for managing gateways. Gateways can be created either
by setting the `--gw-create-on-stats` / `GW_CREATE_ON_STATS` flag, or by using
the [api]({{< ref "integrate/api.md" >}}).

### Gateway location

The (last known) location of the gateway will be stored in the database. When
the gateway is equipped with a GPS, its location will be automatically updated
after every stats update. Else, it can be manually set when creating or
updating the gateway.

### Gateway stats

LoRa Server exposes the gateway statistics on a pre-configured aggregation
intervals (`--gw-stats-aggregation-intervals` / `GW_STATS_AGGREGATION_INTERVALS`). 
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
