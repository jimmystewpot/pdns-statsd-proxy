package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quipo/statsd"
	"go.uber.org/zap"
)

const (
	provider string = "pdns-stats-proxy"
	// metric types
	counterCumulative string = "counter_cumulative"
	gauge             string = "gauge"
)

var (
	log                     = zap.NewExample().Named(provider)
	stats                   *statsd.StatsdBuffer
	gaugeNames              = gaugeMetrics()
	counterCumulativeValues map[string]int64
)

// handle a graceful exit so that we do not lose data when we restart the service.
func watchSignals(sig chan os.Signal, config *Config) {
	for {
		select {
		case <-sig:
			log.Info("Caught signal about to cleanly exit.")
			close(config.Done)
			err := stats.Close()
			if err != nil {
				log.Warn("shutting-down",
					zap.Error(err),
				)
			}
			return
		}
	}
}

func main() {
	config := new(Config)
	if !validateConfiguration(config) {
		log.Fatal("Unable to process configuration, missing flags")
	}

	sigs := make(chan os.Signal, 1)

	// initiate the statsd client.
	var err error
	stats, err = NewStatsClient(config)
	if err != nil {
		log.Fatal("Unable to initiate statsd client")
	}

	counterCumulativeValues = make(map[string]int64)

	// initiate the powerdns client.
	pdnsClient := NewPdnsClient(config)
	// start background worker goroutines.
	go DNSWorker(config, pdnsClient)
	go StatsWorker(config)

	// handle signals correctly.
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL, syscall.SIGTERM)
	go watchSignals(sigs, config)

	// wait until the Done channel is terminated before cleanly exiting.
	<-config.Done
	time.Sleep(5 * time.Second)
}
