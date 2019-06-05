package main

import (
	"fmt"

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
	var statsclient = &statsd.StatsdClient{}
	host := fmt.Sprintf("%s:%d", *config.statsHost, *config.statsPort)

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

		return statsd.NewStatsdBuffer(*config.interval, statsclient), nil
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
					zap.Int("port", *config.statsPort),
					zap.Error(err),
				)
			}
		case <-config.Done:
			log.Warn("done closed, exiting from StatsWorker.")
			return
		}
	}
}

// processStats emits the statistics via the statsd buffer.
func processStats(s Statistic) error {
	switch s.Type {
	case "gauge":
		err := stats.Gauge(s.Name, s.Value)
		if err != nil {
			return err
		}
	case "counter_cumulative": // quipo/statsd supports 'Total', but that does not seem to be standard statsd type
		var err error = nil
		// skip sending first known value for a given incrementing metric because implicit prior value of zero
		// results in ugly data spikes
		if counter_cumulative_values[s.Name] != -1 {
			err = stats.Incr(s.Name, zeroMin(s.Value - counter_cumulative_values[s.Name]))
		}
		counter_cumulative_values[s.Name] = s.Value
		if err != nil {
			return err
		}
	}
	return nil
}

func counterCumulativeMetrics() map[string]int64 {
	counterCumulativeNames := []string{
		"all-outqueries",
		"answers-slow",
		"answers0-1",
		"answers1-10",
		"answers10-100",
		"answers100-1000",
		"auth4-answers-slow",
		"auth4-answers0-1",
		"auth4-answers1-10",
		"auth4-answers10-100",
		"auth4-answers100-1000",
		"auth6-answers-slow",
		"auth6-answers0-1",
		"auth6-answers1-10",
		"auth6-answers10-100",
		"auth6-answers100-1000",
		"cache-hits",
		"cache-misses",
		"case-mismatches",
		"chain-resends",
		"client-parse-errors",
		"dlg-only-drops",
		"dnssec-queries",
		"dnssec-result-bogus",
		"dnssec-result-indeterminate",
		"dnssec-result-insecure",
		"dnssec-result-nta",
		"dnssec-result-secure",
		"dnssec-validations",
		"dont-outqueries",
		"edns-ping-matches",
		"edns-ping-mismatches",
		"ignored-packets",
		"ipv6-outqueries",
		"ipv6-questions",
		"no-packet-error",
		"noedns-outqueries",
		"noerror-answers",
		"noping-outqueries",
		"nsset-invalidations",
		"nxdomain-answers",
		"outgoing-timeouts",
		"outgoing4-timeouts",
		"outgoing6-timeouts",
		"over-capacity-drops",
		"packetcache-hits",
		"packetcache-misses",
		"policy-drops",
		"policy-result-noaction",
		"policy-result-drop",
		"policy-result-nxdomain",
		"policy-result-nodata",
		"policy-result-truncate",
		"policy-result-custom",
		"questions",
		"resource-limits",
		"server-parse-errors",
		"servfail-answers",
		"spoof-prevents",
		"sys-msec",
		"tcp-client-overflow",
		"tcp-outqueries",
		"tcp-questions",
		"throttled-out",
		"throttled-outqueries",
		"too-old-drops",
		"unauthorized-tcp",
		"unauthorized-udp",
		"unexpected-packets",
		"user-msec",
		"unreachables",
	}
	counter_cumulative := make(map[string]int64, len(counterCumulativeNames))
	for _, name := range counterCumulativeNames {
		counter_cumulative[name] = -1
	}
	return counter_cumulative
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
