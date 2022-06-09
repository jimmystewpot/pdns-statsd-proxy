# PowerDNS statistics to statsd bridge

[![Build Status](https://travis-ci.org/jimmystewpot/pdns-statsd-proxy.png?branch=master)](https://travis-ci.org/jimmystewpot/pdns-statsd-proxy) [![codecov](https://codecov.io/gh/jimmystewpot/pdns-statsd-proxy/branch/master/graph/badge.svg)](https://codecov.io/gh/jimmystewpot/pdns-statsd-proxy) [![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=alert_status)](https://sonarcloud.io/dashboard?id=jimmystewpot_pdns-statsd-proxy)[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=jimmystewpot_pdns-statsd-proxy)[![Vulnerabilities](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=vulnerabilities)](https://sonarcloud.io/summary/new_code?id=jimmystewpot_pdns-statsd-proxy)[![Technical Debt](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=sqale_index)](https://sonarcloud.io/summary/new_code?id=jimmystewpot_pdns-statsd-proxy)[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=jimmystewpot_pdns-statsd-proxy&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=jimmystewpot_pdns-statsd-proxy)

Background: [PowerDNS](https://www.powerdns.com/) is a powerful open source DNS server that offers both recursive and authoritative packages. It has powerful statistics available via a HTTP RESTFul API, carbon protocol or via a cli tool. The problem with this is that not all metrics systems provide support for carbon or have http agents that support the PowerDNS API.

This tool aims to provide a lightweight http to statsd bridge/proxy. It will query the PowerDNS API and emit the metrics in either statsd gauge or increments to a statsd server of your choice.

# PowerDNS Support

## recursor

* 4.0
* 4.1
* 4.2
* 4.3

## authoritative

* 4.3


# Build

Requires Docker to be installed as it builds within a container to output binaries in Linux elf format.

```make build```

Will output an artifact to *$PWD/bin* 

# Install

```make install```

Will install the artifact from *$PWD/bin* into /opt/pdns-stats-proxy/ and the systemd unit (service)

# Running

Enable in systemctl

```systemctl enable pdns-stats-proxy```


# Architecture

This tool uses a worker model, a powerdns client will execute and poll, the statistics are then passed via channel to a statistics worker which then emits them via statsd.

## License

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fjimmystewpot%2Fpdns-statsd-proxy.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fjimmystewpot%2Fpdns-statsd-proxy?ref=badge_large)