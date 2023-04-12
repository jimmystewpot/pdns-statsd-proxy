module github.com/jimmystewpot/pdns-statsd-proxy

go 1.19

require (
	github.com/quipo/statsd v0.0.0-20180118161217-3d6a5565f314
	go.uber.org/zap v1.24.0
)

require (
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
)

replace golang.org/x/crypto => golang.org/x/crypto v0.8.0

replace golang.org/x/text => golang.org/x/text v0.9.0

replace golang.org/x/net => golang.org/x/net v0.7.0

replace gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1

replace golang.org/x/sys => golang.org/x/sys v0.1.0 
