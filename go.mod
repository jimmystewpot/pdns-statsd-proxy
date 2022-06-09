module github.com/jimmystewpot/pdns-statsd-proxy

go 1.17

require (
	github.com/quipo/statsd v0.0.0-20180118161217-3d6a5565f314
	go.uber.org/zap v1.21.1-0.20220603173429-e06e09a6d396
)

require (
	github.com/stretchr/testify v1.7.2 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.1-0.20220531191214-aa8f15f0a1ac // indirect
)

replace golang.org/x/text => golang.org/x/text v0.3.7

replace golang.org/x/crypto => golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e

replace golang.org/x/net => golang.org/x/net v0.0.0-20220114011407-0dd24b26b47d

replace github.com/stretchr/testify => github.com/stretchr/testify v1.7.2
