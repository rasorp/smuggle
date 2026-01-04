# Getting Started
To get started with Nomad Pipeline you need a Nomad cluster up and running. If
you don't have one yet, you can follow the
[Nomad Getting Started Guide](https://www.nomadproject.io/intro/getting-started/install)
to set up a local development cluster.

> **Note**
Smuggle in client mode only supports Linux clients and requires root privileges
to run.

## Install Smuggle
Smuggle and the Smuggle CNI plugin can be installed either by building from
source or by downloading pre-built binaries.

### Downloading Pre-built Binaries
You can download pre-built Smuggle binaries from the
[GitHub releases page](https://github.com/rasorp/smuggle/releases) and extract
them to a directory on your system.
```bash
cd /tmp
wget https://github.com/rasorp/smuggle/releases/download/v0.0.1-alpha.1/smuggle_v0.0.1-alpha.1_linux_amd64.tar.gz
tar -xvf smuggle_v0.0.1-alpha.1_linux_amd64.tar.gz
sudo mv smuggle /usr/local/bin/
```

The same process can be used for the Smuggle CNI plugin:
```bash
cd /tmp
wget https://github.com/rasorp/smuggle-cni/releases/download/v0.0.1-alpha.1/smuggle-cni_v0.0.1-alpha.1_linux_amd64.tar.gz
tar -xvf smuggle-cni_v0.0.1-alpha.1_linux_amd64.tar.gz
sudo mv smuggle-cni /opt/cni/bin/smuggle
```

### Building from Source
You can build Smuggle from source by cloning the repository and using Go to
compile it. It will produce a single binary file that you can run within the
`./bin` directory.
```bash
git clone https://github.com/rasorp/smuggle
cd smuggle
make build
```

The same process can be used for the Smuggle CNI plugin:
```bash
git clone https://github.com/rasorp/smuggle-cni
cd smuggle-cni
make build
```
