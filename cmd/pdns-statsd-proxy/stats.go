package main

import (
	"fmt"
	"net"

	"github.com/quipo/statsd"
	"go.uber.org/zap"
)

// Statistic Wrapper struct
type Statistic struct {
	Name  string
	Type  string
	Value int64
}

// Abs function to ensure incrementing counters never submit negative values.
func zeroMin(x int64) int64 {
	if x < 0 {
		return 0
	}
	return x
}

// NewStatsClient creates a buffered statsd client.
func NewStatsClient(config *Config) (*statsd.StatsdBuffer, error) {
	var statsclient = new(statsd.StatsdClient)
	var logger = new(logger)
	host := net.JoinHostPort(*config.statsHost, *config.statsPort)

	if *config.statsHost != "" {
		if *config.recursor {
			statsclient = statsd.NewStatsdClient(host, "powerdns.recursor.")
		} else {
			statsclient = statsd.NewStatsdClient(host, "powerdns.authoritative.")
		}

		err := statsclient.CreateSocket()
		if err != nil {
			log.Fatal("error creating statsd socket",
				zap.Error(err),
			)
		}
		s := statsd.NewStatsdBuffer(*config.interval, statsclient)
		s.Logger = logger

		return s, nil
	}
	// return error
	return &statsd.StatsdBuffer{}, fmt.Errorf("error, unable to create statsd buffer")
}

// StatsWorker wraps a ticker for task execution.
func StatsWorker(config *Config) {
	log.Info("Starting statsd statistics worker...")

	for {
		select {
		case s := <-config.StatsChan:
			err := processStats(s)
			if err != nil {
				log.Warn("error submitting statistics",
					zap.String("host", *config.statsHost),
					zap.String("port", *config.statsPort),
					zap.Error(err),
				)
			}
		case <-config.statsDone:
			err := stats.Close()
			if err != nil {
				log.Warn("unable to cleanly close statsd buffer",
					zap.Error(err),
				)
			}
			log.Warn("exiting from StatsWorker.")
			close(config.statsDone)
			close(config.StatsChan)
			return
		}
	}
}

// processStats emits the statistics via the statsd buffer.
func processStats(s Statistic) error {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				log.Info("recovered from panic in statsd processStats()",
					zap.Error(err),
				)
			}
		}
	}()

	switch s.Type {
	case gauge:
		err := stats.Gauge(s.Name, s.Value)
		if err != nil {
			return err
		}
	case counterCumulative: // quipo/statsd supports 'Total', but that does not seem to be standard statsd type
		var err error = nil
		// skip sending first known value for a given incrementing metric because implicit prior value of zero
		// results in ugly data spikes
		if val, ok := counterCumulativeValues[s.Name]; ok {
			if val != -1 {
				err = stats.Incr(s.Name, zeroMin(s.Value-counterCumulativeValues[s.Name]))
				if err != nil {
					return err
				}
			}
			counterCumulativeValues[s.Name] = s.Value
		}
	}
	return nil
}

func gaugeMetrics() map[string]int {
	gaugeNames := []string{
		"cache-bytes",
		"cache-entries",
		"concurrent-queries",
		"failed-host-entries",
		"malloc-bytes",
		"max-mthread-stack",
		"negcache-entries",
		"nsspeeds-entries",
		"packetcache-entries",
		"packetcache-bytes",
		"qa-latency",
		"security-status",
		"tcp-clients",
		"throttle-entries",
		"uptime",
	}
	gauge := make(map[string]int, len(gaugeNames))
	for _, name := range gaugeNames {
		gauge[name] = 1
	}
	return gauge
}
