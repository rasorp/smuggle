# Configuration: CNI
The Smuggle CNI plugin is triggered by Nomad when a task with CNI networking is
started. The [Smuggle CNI Plugin](https://github.com/rasorp/smuggle-cni) must be
available on each Nomad client node within the
[client.cni_path](https://developer.hashicorp.com/nomad/docs/configuration/client#cni_path)
directory.

The Nomad client agent must be restarted after installing the Smuggle CNI plugin
or updating the CNI configuration files.

## CNI Configuration
Each Nomad client node must have a CNI configuration file for Smuggle within the
[client.cni_config_dir](https://developer.hashicorp.com/nomad/docs/configuration/client#cni_config_dir)
directory. The files must be named with a `.conf` or `.json` suffix and contain
JSON content.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | `vxlan` | Name of the network |
| `type` | string | `smuggle` | CNI plugin name |

### Example
Create a CNI configuration file at `/opt/cni/config/vxlan.conf` with the
following content:
```json
{
  "name": "vxlan",
  "type": "smuggle"
}
```

This configuration will instruct Nomad to use the Smuggle CNI plugin for tasks
and can be used within the Nomad
[network block](https://developer.hashicorp.com/nomad/docs/job-specification/network)
of a job specification:
```hcl
network {
  mode = "cni/vxlan"
}
```

If you are registering services with Nomad or Consul via the Nomad job
[service block](https://developer.hashicorp.com/nomad/docs/job-specification/service),
you may need to specify the `address_mode` to ensure the correct IP address is
used. For example:
```hcl
service {
  name         = "my-service"
  port         = "http"
  address_mode = "alloc"
}
```
