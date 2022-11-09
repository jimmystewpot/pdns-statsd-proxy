module github.com/jimmystewpot/pdns-statsd-proxy

go 1.18

require (
	github.com/quipo/statsd v0.0.0-20180118161217-3d6a5565f314
	go.uber.org/zap v1.23.0
)

require (
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
)

replace golang.org/x/net => golang.org/x/net v0.0.0-20220906165146-f3363e06e74c

replace gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1

replace golang.org/x/crypto => golang.org/x/crypto v0.0.0-20201216223049-8b5274cf687f
