module github.com/jimmystewpot/pdns-statsd-proxy

go 1.16

require (
	github.com/quipo/statsd v0.0.0-20180118161217-3d6a5565f314
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.19.1-0.20210813012313-d8fd848d18f2
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace go.uber.org/goleak => github.com/jimmystewpot/goleak v1.1.11-0.20210813005559-691160354723
