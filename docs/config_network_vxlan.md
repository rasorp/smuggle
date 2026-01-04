# Configuration: Network Provider VXLAN
The VXLAN provider enables
[VXLAN](https://en.wikipedia.org/wiki/Virtual_Extensible_LAN) overlays.

## Config Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `vni` | int | `1` | VXLAN Network Identifier (VNI) to use for the overlay |
| `port` | int | `4789` | UDP port to use for VXLAN traffic |

## Examples
Here is an example network configuration using the VXLAN provider that sets all
the available VXLAN config options:
```json
{
  "name": "vxlan",
  "ipmasq": true,
  "ipv4": {
    "network": "10.10.0.0/16",
    "size": 24
  },
  "provider": {
    "name": "vxlan",
    "config": {
      "vni": 42,
      "port": 4789
    }
  }
}
```

### nvar Configuration Example
When using the Nomad Variables (`nvar`) store backend, create a variable
containing the network configuration JSON. For example:
```console
nomad var put smuggle/networks/v1/vxlan data='{"name":"vxlan","ipv4":{"network":"10.10.0.0/16","size":24},"provider":{"name":"vxlan","config":{"vni":42,"port":4789}}}'
```
