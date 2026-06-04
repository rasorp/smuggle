# Architecture
Smuggle consists of three core components; the Smuggle Client, the Smuggle
Server, and the Smuggle CNI Plugin.

### Smuggle Client
The Smuggle client is a long-running process responsible for managing the host's
networking. It is expected to run on every node in a cluster. All reads and
writes to the backing store are performed via RPC through a Smuggle server. The
client does not communicate with Nomad or the store backend directly.

### Smuggle Server
The Smuggle server is a long-running process that acts as the sole gateway to
the backing store and handles centralised cluster tasks such as reaping expired
subnets. All client subnet and network store operations are proxied through the
server's RPC listener. Each Nomad cluster only needs a single Smuggle server
running.

### Smuggle CNI Plugin
The Smuggle CNI Plugin is a meta plugin responsible for reading configuration
data written by the Smuggle client and delegating to the appropriate underlying
CNI plugin to create the container's network interface. The Smuggle CNI Plugin
is expected to be installed on every node in a cluster.
