module github.com/jimmystewpot/pdns-statsd-proxy

go 1.16

require (
	github.com/quipo/statsd v0.0.0-20180118161217-3d6a5565f314
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.20.0
	golang.org/x/net v0.0.0-20220114011407-0dd24b26b47d // indirect
)

replace golang.org/x/text => golang.org/x/text v0.3.7

replace golang.org/x/crypto => golang.org/x/crypto v0.0.0-20210812204632-0ba0e8f03122
