# Vagrant environment

There is the Vagrant environment for quick start with loraserver infrastructure.
Just install VirtualBox and Vagrant, then run ```vagrant up``` from current directory.
You got Ubuntu based virtual machine with Docker containers and all loraserver components built from source.

```
vagrant@vagrant:~$ docker ps -a
CONTAINER ID        IMAGE                         COMMAND                  CREATED             STATUS              PORTS                    NAMES
f5c179d20710        redis:3.0.7-alpine            "docker-entrypoint.sh"   17 minutes ago      Up 17 minutes       6379/tcp                 vagrant_redis_1
cbcd09d9ffe6        postgres:9.5                  "/docker-entrypoint.s"   17 minutes ago      Up 17 minutes       5432/tcp                 vagrant_postgres_1
4ea2a04a8262        ansi/mosquitto                "/usr/local/sbin/mosq"   17 minutes ago      Up 17 minutes       1883/tcp                 vagrant_mosquitto_1
d67fc719316e        vagrant_lora-gateway-bridge   "/go/src/github.com/b"   17 minutes ago      Up 17 minutes       0.0.0.0:1700->1700/udp   vagrant_lora-gateway-bridge_1
8eb69b6b8738        vagrant_loraserver            "/go/src/github.com/b"   17 minutes ago      Up 17 minutes                                vagrant_loraserver_1
66e44204a7fa        redis:3.0.7-alpine            "docker-entrypoint.sh"   17 minutes ago      Up 17 minutes       6379/tcp                 vagrant_redis_test_1
1365f9092c12        postgres:9.5                  "/docker-entrypoint.s"   17 minutes ago      Up 17 minutes       5432/tcp                 vagrant_postgres_test_1
2162c38425eb        vagrant_lora-app-server       "/go/src/github.com/b"   17 minutes ago      Up 17 minutes       0.0.0.0:8080->8080/tcp   vagrant_lora-app-server_1
```

Please note the environment variables in Vagrantfile#L120 that defines band range and network identifier.
```
BAND=EU_863_870 NET_ID=010203 docker-compose -f /vagrant/docker-compose.yml up -d
```
Check possible values in [loraserver documentation](https://docs.loraserver.io/loraserver/configuration/)
