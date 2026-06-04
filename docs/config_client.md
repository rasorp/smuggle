# Configuration: Client
The Smuggle client supports configuration through command-line flags,
environment variables, and configuration files in HCL or JSON format.

The `config` flag can be used multiple times to load configuration files. Once
all files have been parsed and merged, command-line flags and environment
variables are applied last to override any default values and settings from the
files.

## Client
The client manages local host networking, VXLAN interfaces, and CNI
configurations. All subnet and network store operations are performed via RPC
through the Smuggle server. The client does not connect to Nomad or the backing
store directly.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `data_dir` | string | `/var/lib/smuggle/client` | Directory for client data (CNI configs, agent ID) |
| `disable_ipmasq` | bool | `false` | Disable IP masquerading for container traffic |
| `network_interface` | string | auto-detected | Network interface to use for VXLAN tunnels |

### Command-Line Flags
```bash
--data-dir=/path/to/dir
--disable-ipmasq
--network-interface=eth0
```

### Environment Variables
```bash
SMUGGLE_DATA_DIR=/var/lib/smuggle/client
SMUGGLE_DISABLE_IPMASQ=true
SMUGGLE_NETWORK_INTERFACE=eth0
```

### Configuration File
**HCL:**
```hcl
client {
  data_dir          = "/var/lib/smuggle/client"
  disable_ipmasq    = false
  network_interface = "eth0"
}
```

**JSON:**
```json
{
  "client": {
    "data_dir": "/var/lib/smuggle/client",
    "disable_ipmasq": false,
    "network_interface": "eth0"
  }
}
```

## Servers
The servers block configures the Smuggle server addresses the client connects
to via RPC. All reads and writes to the backing store are proxied through the
server — the client never contacts Nomad or the store backend directly.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `addresses` | list(string) | `["localhost:8081"]` | List of Smuggle server RPC addresses in `host:port` form |

### Command-Line Flags
```bash
--servers=10.0.0.1:8081
--servers=10.0.0.2:8081
```

### Environment Variables
```bash
SMUGGLE_SERVERS=10.0.0.1:8081,10.0.0.2:8081
```

### Configuration File
**HCL:**
```hcl
client {
  servers {
    addresses = ["10.0.0.1:8081", "10.0.0.2:8081"]
  }
}
```

**JSON:**
```json
{
  "client": {
    "servers": {
      "addresses": ["10.0.0.1:8081", "10.0.0.2:8081"]
    }
  }
}
```

## HTTP
The HTTP server exposes a simple health check and optional debugging endpoints.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable HTTP server |
| `address` | string | `localhost` | Address to bind HTTP server |
| `port` | uint | `9090` | Port to bind HTTP server |
| `access_log_level` | string | `debug` | Log level for HTTP access logs |
| `debug_enabled` | bool | `false` | Enable pprof debug endpoints |

### Command-Line Flags
```bash
--http-enabled
--http-address=0.0.0.0
--http-port=8080
--http-access-log-level=info
--http-debug-enabled
```

### Environment Variables
```bash
SMUGGLE_HTTP_ENABLED=true
SMUGGLE_HTTP_ADDRESS=0.0.0.0
SMUGGLE_HTTP_PORT=8080
SMUGGLE_HTTP_ACCESS_LOG_LEVEL=info
SMUGGLE_HTTP_ENABLE_DEBUG=true
```

### Configuration File
**HCL:**
```hcl
http {
  enabled          = true
  address          = "0.0.0.0"
  port             = 8080
  access_log_level = "info"
  debug_enabled    = false
}
```

**JSON:**
```json
{
  "http": {
    "enabled": true,
    "address": "0.0.0.0",
    "port": 8080,
    "access_log_level": "info",
    "debug_enabled": false
  }
}
```

## Logging
Configure the client log output format and verbosity.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `level` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `file` | string | `""` | Absolute path to a file to write logs to; logs are also always written to stdout. The file is rotated at 100MB, with 5 backups kept for 30 days |
| `json` | bool | `false` | Output logs in JSON format |
| `include_line` | bool | `false` | Include file and line numbers in logs |
| `enable_stacktrace` | bool | `false` | Include stack traces for errors |

### Command-Line Flags
```bash
--log-level=debug
--log-file=/var/log/smuggle/client.log
--log-json
--log-include-line
--log-enable-stacktrace
```

### Environment Variables
```bash
SMUGGLE_LOG_LEVEL=debug
SMUGGLE_LOG_FILE=/var/log/smuggle/client.log
SMUGGLE_LOG_JSON=true
SMUGGLE_LOG_INCLUDE_LINE=true
SMUGGLE_LOG_ENABLE_STACKTRACE=true
```

### Configuration File
**HCL:**
```hcl
log {
  level             = "debug"
  file              = "/var/log/smuggle/client.log"
  json              = false
  include_line      = true
  enable_stacktrace = false
}
```

**JSON:**
```json
{
  "log": {
    "level": "debug",
    "file": "/var/log/smuggle/client.log",
    "json": false,
    "include_line": true,
    "enable_stacktrace": false
  }
}
```
