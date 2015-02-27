# WatchDNS

A dynamic DNS responder for fleet clusters.

## Features

- SRV records
- A records (obviously)
- TCP health checks
- HTTP health checks
- Configuration via fleet services (in systemd unit files)
- DNS lookup for `services` and `machines` (by fleet ID and short ID -- used in SRV records)
- Round robin, random, or default record sorting (DNS responses)

Planned:

- Fallback/backup support (only respond with fallbacks/backups if primaries are down)
- Distributed health-checking


## Configuration

All configuration options may be set in one of 3 ways:

- command line flag
- environmental variable
- set in /etc/watchdns/config.toml

`watchdns` uses [viper](https://github.com/spf13/viper) and [cobra](https://github.com/spf13/cobra) for configuration.
You may use any combination of flags (*--key-name*), `env` vars (*WDNS_KEYNAME*), and/or configuration files in `/etc/watchdns/config.*` (*[TOML](https://github.com/toml-lang/toml), [JSON](http://en.wikipedia.org/wiki/JSON), and [YAML](http://en.wikipedia.org/wiki/YAML) supported*)

You can also run `watchdns --help` for a list of options and more details about what they do.

```toml
# All config keys and their defaults
Domain="watchdns."
CheckInterval="5s"
EtcdPeers="http://localhost:4001"
FleetInterval="10s"
FleetPrefix=""
BindAddress=":8053"
LogLevel="info"
LogFormat="ascii"
```
