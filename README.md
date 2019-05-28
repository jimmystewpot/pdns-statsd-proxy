# powerdns to statsd bridge

Background: [PowerDNS](https://www.powerdns.com/) is a powerful open source DNS server that offers both recursive and authoritative packages. It has powerful statistics available via a HTTP RESTFul API, carbon protocol or via a cli tool. The problem with this is that not all metrics systems provide support for carbon or have http agents that support the PowerDNS API.

This tool aims to provide a lightweight http to statsd bridge/proxy. It will query the PowerDNS API and emit the metrics in either statsd gauge or increments to a statsd server of your choice.

# Build

```make build```

Will output an artifact to *$PWD/bin* 

# Install

```make install```

Will install the artifact from *$PWD/bin* into /opt/pdns-stats-proxy/ and the systemd unit (service)

# Running

enable in systemctl

```systemctl enable pdns-stats-proxy```