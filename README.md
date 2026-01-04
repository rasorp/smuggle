# Smuggle
Smuggle is a lightweight layer 3 overlay network fabric for
[IBM HashiCorp Nomad](https://www.nomadproject.io/). It currently supports
[VXLAN](https://en.wikipedia.org/wiki/Virtual_Extensible_LAN) overlays.

While other container networking solutions exist, most are focused on
Kubernetes and are either incompatible with Nomad or require additional
services to be run which are not typically part of a Nomad deployment. Smuggle
is designed specifically for Nomad and aims to be simple to deploy and operate
without additional required dependencies.

### Docs
The documentation for Smuggle can be found within the [docs](./docs) directory.
