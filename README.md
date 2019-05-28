# powerdns to statsd bridge

This is a simple PowerDNS statistics to statsd bridge. It queries the statistics API on port 8080 and outputs to statsd on port 8125.

# Build

```make build```

Will output an artifact to *$PWD/bin* 

# Install

```make install```

Will install the artifact from *$PWD/bin* into /opt/pdns-stats-proxy/ and the systemd unit (service)

# Running

enable in systemctl

```systemctl enable pdns-stats-proxy```