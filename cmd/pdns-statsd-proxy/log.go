package main

import (
	"go.uber.org/zap"
)

// Logger replace the statsd default logger from Println to zap
//
//nolint:nolintlint //Logger is used to implement the logger interface for statsd.
type Logger interface {
	Println(v ...any)
}

type logger struct{}

var _ Logger = (*logger)(nil)
var _ = (*logger).Println

func (l *logger) Println(v ...any) {
	for _, entry := range v {
		if val, ok := entry.(string); ok {
			log.Info("BufferedStatsdClient",
				zap.String("result", val))
		}
	}
}

// initialise the log global variable for logging.
func initLogger() error {
	logger, err := zap.NewProduction(zap.AddCaller())
	if err != nil {
		return err
	}
	// set the global var log to the zap logger.
	log = logger.Named(provider)
	return nil
}
