# HTTP API
The Smuggle HTTP API allows you to interact with the Smuggle service
programmatically. Below are the available endpoints and their descriptions.

All endpoints are prefixed with `/v1/` to indicate the current and initial API
version.

## `system/health` Endpoint
The `system/health` endpoint provides a basic health check endpoint for the
Smuggle agent.

### Example Usage
To check the health of the Smuggle agent, you can use the following curl command:
```bash
$ curl http://localhost:9090/v1/system/health
{"status":"OK","message":"Smuggle agent is healthy"}
```

## `debug/pprof` Endpoint
The `debug/pprof` endpoint provides optional access to pprof profiling data for
performance analysis and debugging.

- `GET /v1/debug/pprof/`: Lists available pprof profiles.
- `GET /v1/debug/pprof/{profile}`: Retrieves the specified pprof profile.
- `GET /v1/debug/pprof/cmdline`: Returns the command line arguments used to start Smuggle.
- `GET /v1/debug/pprof/profile`: Captures a 30-second CPU profile.
- `GET /v1/debug/pprof/symbol`: Looks up program counters to symbol names.
- `GET /v1/debug/pprof/trace`: Captures a 1-second execution trace.

### Example Usage
To capture a CPU profile, you can use the following curl command:
```bash
$ curl http://localhost:9090/v1/debug/pprof/profile?seconds=5 -o cpu_profile.out
```
