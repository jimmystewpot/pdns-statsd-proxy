package main

import (
	"go.uber.org/zap"
)

// Logger replace the statsd default logger from Println to zap
// nolint
type Logger interface {
	Println(v ...interface{})
}

type logger struct{}

func (l *logger) Println(v ...interface{}) {
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
