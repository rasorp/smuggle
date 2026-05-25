# Architecture
Smuggle consists of three core components; the Smuggle Client, the Smuggle
Server, and the Smuggle CNI Plugin.

### Smuggle Client
The Smuggle client is a long-running process responsible for managing the
host's networking. It is expected to run on every node in a cluster.

### Smuggle Server
The Smuggle server is a long-running process responsible for reaping networks
that have expired. Each Nomad cluster only needs a single Smuggle server
running.

### Smuggle CNI Plugin
The Smuggle CNI Plugin is a meta plugin responsible for reading configuration
data written by the Smuggle client and delegating to the appropriate underlying
CNI plugin to create the container's network interface. The Smuggle CNI Plugin
is expected to be installed on every node in a cluster.
