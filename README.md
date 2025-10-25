# opsproxy

Small reverse proxy. This repository contains a tiny Go app and a Taskfile to build multi-platform binaries.

Builds
------

Use the included Taskfile with task (https://taskfile.dev/) to build artifacts into `dist/`.

- Build for current host:

````markdown
# opsproxy

Small reverse proxy. This repository contains a tiny Go app and a Taskfile to build multi-platform binaries.

Builds
------

Use the included Taskfile with task (https://taskfile.dev/) to build artifacts into `dist/`.

- Build for current host:

```bash
task build
```

- Build all supported platforms:

```bash
task build:all
```

Outputs are placed in `dist/` with names like `opsproxy-darwin-arm64`, `opsproxy-linux-amd64`, or `opsproxy-windows-amd64.exe`.

````

## Flags and CLI options

The `opsproxy` binary accepts a few simple flags to control where it listens and where it forwards traffic.

### Flags

- `--listen-port` (int, default: 80)
	- Port for the proxy to listen on. Use a port >=1024 for non-root processes (for example, `8080`).

- `--target-host` (string, default: `127.0.0.1`)
	- The upstream host to forward requests to. This may include a scheme (`http://` or `https://`) and/or a port. If no scheme is provided the proxy assumes `http://`.
	- If `target-host` already contains a port, the `--target-port` value will not be appended.

- `--target-port` (int, default: 8080)
	- The port to use when `--target-host` does not include a port.

- `--log-level` (string, default: `info`)
	- Controls logging verbosity. Supported values:
		- `verbose` — log every forwarded request (method, URL and remote address).
		- `info` — log only the first request (useful for confirming traffic is flowing without noisy logs).
		- `error` — only log proxy errors (backend failures). Unknown values fall back to `info`-like behavior.

### Behavior notes

- On backend errors the proxy returns HTTP 502 (Bad Gateway) and logs the error.
- When you provide a full URL (for example `https://example.com:8443`) as `--target-host`, the proxy will use that exact URL and will not append `--target-port`.

### Examples

- Run the built binary on macOS/Linux (adjust binary name for your platform):

```bash
./dist/opsproxy-darwin-arm64 --listen-port=8080 --target-host=127.0.0.1 --target-port=8080 --log-level=verbose
```

- Run on Windows (PowerShell):

```powershell
.\dist\opsproxy-windows-amd64.exe --listen-port=8080 --target-host=127.0.0.1 --target-port=8080 --log-level=info
```

- Use the Taskfile to build and run the proxy (cross-platform):

```bash
task build   # build for current host
task run     # build if needed and start the proxy
```

If you prefer to only build and not start the binary, run `task build` and then run the binary manually (as shown above).
