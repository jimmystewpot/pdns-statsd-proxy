package main

import (
	"fmt"
	"net"

	ddstatsd "github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/zap"
)

type statsClient interface {
	Gauge(name string, value float64, tags []string, rate float64) error
	Count(name string, value int64, tags []string, rate float64) error
	Close() error
}

var _ = gaugeMetrics

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

// NewStatsClient creates a statsd client.

func NewStatsClient(config *Config) (statsClient, error) {
	host := net.JoinHostPort(*config.statsHost, *config.statsPort)

	if *config.statsHost != "" {
		namespace := "powerdns.authoritative."
		if *config.recursor {
			namespace = "powerdns.recursor."
		}

		client, err := ddstatsd.New(host, ddstatsd.WithNamespace(namespace))
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	// return error
	return nil, fmt.Errorf("error, no statsd host configured")
}

// statsWorker wraps a ticker for task execution.
func statsWorker(config *Config) {
	log.Info("Starting statsd statistics worker...")
	defer close(config.statsExited)

	for {
		select {
		case s, ok := <-config.StatsChan:
			if !ok {
				log.Info("exiting from StatsWorker.")
				return
			}
			err := processStats(s, config.counterCumulativeValues)
			if err != nil {
				log.Error("error submitting statistics",
					zap.String("metric_name", s.Name),
					zap.String("host", *config.statsHost),
					zap.String("port", *config.statsPort),
					zap.Error(err),
				)
			}
		case <-config.stop:
			err := stats.Close()
			if err != nil {
				log.Error("unable to cleanly close statsd buffer",
					zap.Error(err),
				)
			}
			log.Info("exiting from StatsWorker.")
			return
		}
	}
}

// processStats emits the statistics via the statsd buffer.
func processStats(s Statistic, counterCumulativeValues map[string]int64) error {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				log.Error("recovered from panic in statsd processStats()",
					zap.Error(err),
				)
			}
		}
	}()

	switch s.Type {
	case gauge:
		err := stats.Gauge(s.Name, float64(s.Value), nil, 1)
		if err != nil {
			return err
		}
	case counterCumulative:
		// skip sending first known value for a given incrementing metric because implicit prior value of zero
		// results in ugly data spikes
		if val, ok := counterCumulativeValues[s.Name]; ok {
			if val != -1 {
				err := stats.Count(s.Name, zeroMin(s.Value-counterCumulativeValues[s.Name]), nil, 1)
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
