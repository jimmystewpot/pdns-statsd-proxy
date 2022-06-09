module github.com/jimmystewpot/pdns-statsd-proxy

go 1.17

require (
	github.com/quipo/statsd v0.0.0-20180118161217-3d6a5565f314
	go.uber.org/zap v1.21.0
)

require (
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace golang.org/x/text => golang.org/x/text v0.3.7

replace golang.org/x/crypto => golang.org/x/crypto v0.0.0-20210812204632-0ba0e8f03122

replace golang.org/x/net => golang.org/x/net v0.0.0-20220114011407-0dd24b26b47d
