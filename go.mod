module github.com/jimmystewpot/pdns-statsd-proxy

go 1.18

require (
	github.com/quipo/statsd v0.0.0-20180118161217-3d6a5565f314
	go.uber.org/zap v1.24.0
)

require (
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
)

replace golang.org/x/text/encoding/unicode => golang.org/x/text/encoding/unicode v0.3.7

replace golang.org/x/crypto => golang.org/x/crypto v0.0.0-20220518034528-6f7dac969898

replace golang.org/x/text/internal/language => golang.org/x/text/internal/language v0.3.7

replace golang.org/x/net/http2 => golang.org/x/net/http2 v0.0.0-20220517181318-183a9ca12b87
