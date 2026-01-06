# Smuggle Documentation
This documentation provides an overview of Smuggle, including its architecture,
API, and configuration options.

## Table of Contents
- [Architecture](./_architecture.md)
- [Getting Started](./_getting_started.md)
- Configuration Options
  - [Agent](./config_agent.md)
  - [CNI Plugin](./config_cni.md)
  - [Network](./config_network.md)
  - [Network Provider VXLAN](./config_network_vxlan.md)
- [API](./api.md)
- [Troubleshooting](./troubleshooting.md)

## Known Issues
* Ubuntu 22.04 and later has a known issue within the netlink library tracked as
  [issue #993](https://github.com/vishvananda/netlink/issues/993) where a race
  condition can lead to inconsistent Mac address detection. It can be avoid by
  removing the `/usr/lib/systemd/network/99-default.link` file.
