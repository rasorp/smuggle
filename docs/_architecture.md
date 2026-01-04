# Architecture
Smuggle consists of two core conponents; the Smuggle Agent and the Smuggle
CNI Plugin.

### Smuggle Agent
The Smuggle agent is a long-running process that can be run in client or server
mode. The Smuggle agent in client mode is responsible for managing the host's
networking and is expected to run on every node in a cluster. The Smuggle agent
in server mode is responsible for reaping networks that have expired and each
Nomad cluster only needs a single Smuggle agent running in server mode.

### Smuggle CNI
The Smuggle CNI Plugin is a meta plugin responsible for reading configuration
data written by the Smuggle agent and delegating to the appropriate underlying
CNI plugin to create the container's network interface. The Smuggle CNI Plugin
is expected to be installed on every node in a cluster.
