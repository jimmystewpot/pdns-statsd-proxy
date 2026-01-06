# PowerDNS statistics to statsd bridge

[![codecov](https://codecov.io/gh/jimmystewpot/pdns-statsd-proxy/branch/master/graph/badge.svg)](https://codecov.io/gh/jimmystewpot/pdns-statsd-proxy) [![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=jimmystewpot_pdns-statsd-proxy)[![Vulnerabilities](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=vulnerabilities)](https://sonarcloud.io/summary/new_code?id=jimmystewpot_pdns-statsd-proxy)[![Technical Debt](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=sqale_index)](https://sonarcloud.io/summary/new_code?id=jimmystewpot_pdns-statsd-proxy)[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=jimmystewpot_pdns-statsd-proxy)

Background: [PowerDNS](https://www.powerdns.com/) is a powerful open source DNS server that offers both recursive and authoritative packages. It has powerful statistics available via a HTTP RESTFul API, carbon protocol or via a cli tool. The problem with this is that not all metrics systems provide support for carbon or have http agents that support the PowerDNS API.

This tool aims to provide a lightweight http to statsd bridge/proxy. It will query the PowerDNS API and emit the metrics in either statsd gauge or increments to a statsd server of your choice.

# How it works

This tool runs two background workers:

- **PowerDNS poller**: polls the PowerDNS HTTP API on an interval.
- **Statsd emitter**: receives decoded metrics via a channel and emits them to statsd.

## PowerDNS API mode selection

PowerDNS Recursor added a Prometheus-compatible `/metrics` endpoint in **4.3.0**.

To support both API styles without requiring additional configuration, the client performs version discovery:

- **On the first successful poll**, the client requests the legacy JSON statistics endpoint and reads the `Server:` header (for example `PowerDNS/4.9.0`).
- The parsed version is **cached** and does not need to be re-evaluated on subsequent polls.
- Based on the discovered version:
  - **`4.0 <= version < 4.3`**: uses the legacy JSON endpoint.
  - **`version >= 4.3`**: uses the Prometheus `/metrics` endpoint.

# PowerDNS Support

## recursor

- 4.0, 4.1, 4.2: `/api/v1/servers/localhost/statistics`
- 4.3+: `/metrics`

## authoritative

- 4.3+: `/metrics` (requires the authoritative server webserver/API to be enabled)

# Configuration

## Flags

- `-statsHost` (default `127.0.0.1`): statsd host
- `-statsPort` (default `8125`): statsd port
- `-pdnsHost` (default `127.0.0.1`): PowerDNS webserver/API host
- `-pdnsPort` (default `8080`): PowerDNS webserver/API port
- `-key` (default empty): PowerDNS API key (sent as `X-API-Key`)
- `-recursor` (default `true`): whether to prefix metrics as recursor vs authoritative
- `-interval` (default `15s`): polling interval

## Environment variables

- `PDNS_API_KEY`: used as a fallback if `-key` is not set or is empty.

# Running

## Example: recursor

```bash
PDNS_API_KEY=changeme \
  ./pdns-statsd-proxy \
  -pdnsHost 127.0.0.1 \
  -pdnsPort 8082 \
  -statsHost 127.0.0.1 \
  -statsPort 8125 \
  -recursor=true \
  -interval 15s
```

## Example: authoritative

```bash
PDNS_API_KEY=changeme \
  ./pdns-statsd-proxy \
  -pdnsHost 127.0.0.1 \
  -pdnsPort 8081 \
  -statsHost 127.0.0.1 \
  -statsPort 8125 \
  -recursor=false \
  -interval 15s
```

# Testing

## Unit tests

```bash
go test ./...
```

## Docker-backed contract tests

Contract tests spin up real PowerDNS containers via Docker to validate behavior against real services.

They are opt-in and require:

- Docker available
- `PDNS_CONTRACT=1`

Run:

```bash
PDNS_CONTRACT=1 go test -tags contract ./...
```

Optional overrides:

- `PDNS_RECURSOR_PRE43_IMAGE` (default `powerdns/pdns-recursor-42:4.2.0`)
- `PDNS_RECURSOR_43PLUS_IMAGE` (default `powerdns/pdns-recursor-49:4.9.0`)

# Build

You can build locally with Go or use the provided Docker-based Makefile.

## Local build

```bash
go build -o bin/pdns-statsd-proxy ./cmd/pdns-statsd-proxy
```

## Makefile build

Requires Docker to be installed as it builds within a container to output binaries in Linux elf format.

```bash
make build
```

Will output an artifact to *$PWD/bin*

# Install

## From GitHub Releases

GitHub Releases publish pre-built artifacts for common OS/architecture combinations:

- `.deb` (Debian/Ubuntu)
- `.rpm` (RHEL/Fedora/Rocky/Alma/SUSE)
- `.tar.gz` (generic archive containing the binary)

Pick the correct asset for your platform from:

https://github.com/jimmystewpot/pdns-statsd-proxy/releases

### Debian/Ubuntu (`.deb`)

```bash
sudo dpkg -i ./pdns-statsd-proxy_<version>_linux_<arch>.deb
sudo systemctl daemon-reload
sudo systemctl enable --now pdns-statsd-proxy
```

### RHEL/Fedora/Rocky/Alma/SUSE (`.rpm`)

```bash
sudo rpm -Uvh ./pdns-statsd-proxy_<version>_linux_<arch>.rpm
sudo systemctl daemon-reload
sudo systemctl enable --now pdns-statsd-proxy
```

### Generic Linux (`.tar.gz`)

```bash
tar -xzf ./pdns-statsd-proxy_<version>_linux_<arch>.tar.gz
sudo install -m 0755 pdns-statsd-proxy /usr/local/bin/pdns-statsd-proxy
pdns-statsd-proxy -h
```

If you want systemd integration when installing from the archive, use the packaged unit file:

```bash
sudo install -m 0644 systemd/pdns-statsd-proxy.service /etc/systemd/system/pdns-statsd-proxy.service
sudo systemctl daemon-reload
sudo systemctl enable --now pdns-statsd-proxy
```

```bash
make install
```

Will install the artifact from *$PWD/bin* into /opt/pdns-stats-proxy/ and the systemd unit (service)

# Service (systemd)

Enable the service:

```bash
systemctl enable pdns-stats-proxy
```


# Architecture

This tool uses a worker model, a powerdns client will execute and poll, the statistics are then passed via channel to a statistics worker which then emits them via statsd.

## License

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fjimmystewpot%2Fpdns-statsd-proxy.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fjimmystewpot%2Fpdns-statsd-proxy?ref=badge_large)