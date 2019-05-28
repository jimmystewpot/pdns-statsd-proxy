package main

import (
	"fmt"

	"github.com/quipo/statsd"
	"go.uber.org/zap"
)

// Statistic Wrapper struct
type Statistic struct {
	Name  string
	Value int64
	Type  string
}

// NewStatsClient creates a buffered statsd client.
func NewStatsClient(config *Config) (*statsd.StatsdBuffer, error) {
	var statsclient = &statsd.StatsdClient{}
	host := fmt.Sprintf("%s:%d", *config.statsHost, *config.statsPort)

	if *config.statsHost != "" {
		if *config.recursor {
			statsclient = statsd.NewStatsdClient(host, "powerdns.recursor")
		} else {
			statsclient = statsd.NewStatsdClient(host, "powerdns.authoritative")
		}

		err := statsclient.CreateSocket()
		if err != nil {
			log.Fatal("error creating statsd socket",
				zap.Error(err),
			)
		}

		return statsd.NewStatsdBuffer(*config.interval, statsclient), nil
	}
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
	case "rate":
		err := stats.Incr(s.Name, s.Value)
		if err != nil {
			return err
		}
	}
	return nil
}

func rateMetrics() map[string]int {
	rateNames := []string{
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
		"cache-bytes",
		"cache-hits",
		"cache-misses",
		"case-mismatches",
		"chain-resends",
		"client-parse-errors",
		"concurrent-queries",
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
		"malloc-bytes",
		"max-mthread-stack",
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
		"packetcache-bytes",
		"packetcache-hits",
		"packetcache-misses",
		"policy-drops",
		"policy-result-noaction",
		"policy-result-drop",
		"policy-result-nxdomain",
		"policy-result-nodata",
		"policy-result-truncate",
		"policy-result-custom",
		"qa-latency",
		"questions",
		"resource-limits",
		"security-status",
		"server-parse-errors",
		"servfail-answers",
		"spoof-prevents",
		"sys-msec",
		"tcp-client-overflow",
		"tcp-clients",
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
	rate := make(map[string]int, len(rateNames))
	for _, name := range rateNames {
		rate[name] = 1
	}
	return rate
}

func gaugeMetrics() map[string]int {
	gaugeNames := []string{
		"cache-entries",
		"failed-host-entries",
		"negcache-entries",
		"nsspeeds-entries",
		"packetcache-entries",
		"throttle-entries",
		"uptime",
	}
	gauge := make(map[string]int, len(gaugeNames))
	for _, name := range gaugeNames {
		gauge[name] = 1
	}
	return gauge
}
