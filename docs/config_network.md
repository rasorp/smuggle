# Configuration: Network
The network configuration object instructs Smuggle to configure and allocate the
specificed network on the host machine. Each host can support multiple networks.

## Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | _required_ | Name of the network |
| `ipmasq ` | bool | `true` | Whether IP masquerading for the network is enabled |
| `ipv4.network` | string | _required_ | IPv4 network CIDR for the overlay (e.g. `10.10.0.0/16`) |
| `ipv4.min` | string | `""` | Minimum allocatable IPv4 address from the network |
| `ipv4.max` | string | `""` | Maximum allocatable IPv4 address from the network |
| `ipv4.size` | int | _required_ | Size of individual client subnets (e.g. `24` for `/24` subnets) |
| `provider.name` | string | _required_ | Name of the network provider to use (e.g. `vxlan`) |
| `provider.config` | json | `{}` | Config options to pass to the network provider |

## Examples
Here is an example network configuration using the VXLAN provider:
```json
{
  "name": "vxlan",
  "ipmasq": true,
  "ipv4": {
    "network": "10.10.0.0/16",
    "size": 24
  },
  "provider": {
    "name": "vxlan"
  }
}
```

### nvar Configuration Example
When using the Nomad Variables (`nvar`) store backend, create a variable
containing the network configuration JSON. For example:
```console
nomad var put smuggle/networks/v1/vxlan data='{"name":"vxlan","ipv4":{"network":"10.10.0.0/16","size":24},"provider":{"name":"vxlan"}}'
```
