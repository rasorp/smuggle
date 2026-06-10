# Configuration: Network Provider WireGuard
The WireGuard provider enables [WireGuard](https://www.wireguard.com/) overlays.

## Config Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `listen_port` | int | `0` | UDP port for the WireGuard interface to listen on. When set to `0` the kernel assigns a free ephemeral port |
| `persistent_keepalive` | int | `0` | Interval in seconds at which WireGuard sends keepalive packets to each peer. Required to maintain UDP mappings for peers behind NAT. A value of `0` disables keepalives |

## Examples
Here is an example network configuration using the WireGuard provider that sets
all the available WireGuard config options:
```json
{
  "name": "wireguard",
  "ipmasq": true,
  "ipv4": {
    "network": "10.20.0.0/16",
    "size": 24
  },
  "provider": {
    "name": "wireguard",
    "config": {
      "listen_port": 51820,
      "persistent_keepalive": 25
    }
  }
}
```

### nvar Configuration Example
When using the Nomad Variables (`nvar`) store backend, create a variable
containing the network configuration JSON. For example:
```console
nomad var put smuggle/networks/v1/wireguard data='{"name":"wireguard","ipv4":{"network":"10.20.0.0/16","size":24},"provider":{"name":"wireguard","config":{"listen_port":51820,"persistent_keepalive":25}}}'
```
