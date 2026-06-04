# Configuration: Server
The Smuggle server supports configuration through command-line flags,
environment variables, and configuration files in HCL or JSON format.

The `config` flag can be used multiple times to load configuration files. Once
all files have been parsed and merged, command-line flags and environment
variables are applied last to override any default values and settings from the
files.

## Server
The server runs centralised tasks for the Smuggle cluster, including reaping
expired subnets. The server also acts as the sole gateway to the backing store; 
all client reads and writes are proxied through the server's RPC listener.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `reaper.interval` | duration | `5m` | Interval between reaper runs |
| `reaper.threshold` | duration | `5m` | Age threshold for removing expired subnets |

### Command-Line Flags
```bash
--reaper-interval=10m
--reaper-threshold=15m
```

### Environment Variables
```bash
SMUGGLE_REAPER_INTERVAL=10m
SMUGGLE_REAPER_THRESHOLD=15m
```

### Configuration File
**HCL:**
```hcl
server {
  reaper {
    interval  = "10m"
    threshold = "15m"
  }
}
```

**JSON:**
```json
{
  "server": {
    "reaper": {
      "interval": "10m",
      "threshold": "15m"
    }
  }
}
```

## RPC
The RPC block configures the server-side RPC listener. Clients connect here for
all subnet and network store operations.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `access_log_level` | string | `debug` | Log level for successful RPC call entries |
| `bind_address` | string | `localhost` | Address to bind the RPC listener |
| `bind_port` | uint | `8081` | Port to bind the RPC listener |
| `write_rate_limit` | int | `20` | Maximum write operations per second forwarded to the backing store (0 = unlimited) |
| `write_burst` | int | `5` | Token-bucket burst size for the write rate limiter |

### Command-Line Flags
```bash
--rpc-access-log-level=info
--rpc-bind-address=0.0.0.0
--rpc-bind-port=8081
--rpc-write-rate=20
--rpc-write-burst=5
```

### Environment Variables
```bash
SMUGGLE_RPC_ACCESS_LOG_LEVEL=info
SMUGGLE_RPC_BIND_ADDRESS=0.0.0.0
SMUGGLE_RPC_BIND_PORT=8081
SMUGGLE_RPC_WRITE_RATE=20
SMUGGLE_RPC_WRITE_BURST=5
```

### Configuration File
**HCL:**
```hcl
server {
  rpc {
    access_log_level = "info"
    bind_address     = "0.0.0.0"
    bind_port        = 8081
    write_rate_limit = 20
    write_burst      = 5
  }
}
```

**JSON:**
```json
{
  "server": {
    "rpc": {
      "access_log_level": "info",
      "bind_address": "0.0.0.0",
      "bind_port": 8081,
      "write_rate_limit": 20,
      "write_burst": 5
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
Configure the server log output format and verbosity.

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
--log-file=/var/log/smuggle/server.log
--log-json
--log-include-line
--log-enable-stacktrace
```

### Environment Variables
```bash
SMUGGLE_LOG_LEVEL=debug
SMUGGLE_LOG_FILE=/var/log/smuggle/server.log
SMUGGLE_LOG_JSON=true
SMUGGLE_LOG_INCLUDE_LINE=true
SMUGGLE_LOG_ENABLE_STACKTRACE=true
```

### Configuration File
**HCL:**
```hcl
log {
  level             = "debug"
  file              = "/var/log/smuggle/server.log"
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
    "file": "/var/log/smuggle/server.log",
    "json": false,
    "include_line": true,
    "enable_stacktrace": false
  }
}
```

## Nomad
Configure connection to the Nomad cluster. This is used to read network
configurations and manage subnet allocations.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `address` | string | `http://localhost:4646` | Nomad API address |
| `token` | string | `""` | Nomad ACL token |
| `ca_cert` | string | `""` | Path to CA certificate for TLS |
| `ca_path` | string | `""` | Path to directory of CA certificates |
| `client_cert` | string | `""` | Path to client certificate for mTLS |
| `client_key` | string | `""` | Path to client private key for mTLS |
| `tls_server_name` | string | `""` | SNI hostname for TLS connection |
| `skip_verify` | bool | `false` | Skip TLS certificate verification |

### Command-Line Flags
```bash
--nomad-addr=https://nomad.example.com:4646
--nomad-token=abc123
--nomad-ca-cert=/etc/nomad/ca.pem
--nomad-client-cert=/etc/nomad/client.pem
--nomad-client-key=/etc/nomad/client-key.pem
--nomad-tls-server-name=nomad.example.com
--nomad-skip-verify
```

### Environment Variables
```bash
NOMAD_ADDR=https://nomad.example.com:4646
NOMAD_TOKEN=abc123
NOMAD_CACERT=/etc/nomad/ca.pem
NOMAD_CAPATH=/etc/nomad/ca-dir
NOMAD_CLIENT_CERT=/etc/nomad/client.pem
NOMAD_CLIENT_KEY=/etc/nomad/client-key.pem
NOMAD_TLS_SERVER_NAME=nomad.example.com
NOMAD_SKIP_VERIFY=false
```

### Configuration File
**HCL:**
```hcl
nomad {
  address         = "https://nomad.example.com:4646"
  token           = "abc123"
  ca_cert         = "/etc/nomad/ca.pem"
  client_cert     = "/etc/nomad/client.pem"
  client_key      = "/etc/nomad/client-key.pem"
  tls_server_name = "nomad.example.com"
  skip_verify     = false
}
```

**JSON:**
```json
{
  "nomad": {
    "address": "https://nomad.example.com:4646",
    "token": "abc123",
    "ca_cert": "/etc/nomad/ca.pem",
    "client_cert": "/etc/nomad/client.pem",
    "client_key": "/etc/nomad/client-key.pem",
    "tls_server_name": "nomad.example.com",
    "skip_verify": false
  }
}
```

## Store
Configure the backend for reading network configuration data and managing subnet
allocations. Currently, only Nomad Variables (`nvar`) backend is supported.

### Options
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `backend` | string | `nvar` | Storage backend type (currently only `nvar`) |
| `nvar.path` | string | `smuggle/` | Path prefix in Nomad Variables |

### Command-Line Flags
```bash
--store-backend=nvar
--store-nvar-path=smuggle/
```

### Environment Variables
```bash
SMUGGLE_STORE_BACKEND=nvar
SMUGGLE_STORE_NVAR_PATH=smuggle/
```

### Configuration File
**HCL:**
```hcl
store {
  backend = "nvar"

  nvar {
    path = "smuggle/"
  }
}
```

**JSON:**
```json
{
  "store": {
    "backend": "nvar",
    "nvar": {
      "path": "smuggle/"
    }
  }
}
```
