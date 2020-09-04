package main

import (
	"fmt"
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
	log                     *zap.Logger
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
			config.pdnsDone <- true  // close downt he pdns worker first
			config.statsDone <- true // close down the statsd worker
			time.Sleep(time.Duration(1) * time.Second)
			close(config.Done) // unblock the main func for a clean exit.
			return
		}
	}
}

func main() {
	err := initLogger()
	if err != nil {
		fmt.Println("unable to initalise logging: ", err)
		os.Exit(1)
	}

	config := new(Config)
	if !validateConfiguration(config) {
		log.Fatal("Unable to process configuration, missing flags")
	}

	// initiate the statsd client.
	stats, err = NewStatsClient(config)
	if err != nil {
		log.Fatal("Unable to initiate statsd client")
	}

	// initiate the powerdns client.
	pdnsClient := new(pdnsClient)
	err = pdnsClient.Initialise(config)
	if err != nil {
		log.Fatal("unable to initialise powerdns client",
			zap.Error(err),
		)
	}
	// start background worker goroutines.
	go pdnsClient.Worker(config)
	go StatsWorker(config)

	// handle signals correctly.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL, syscall.SIGTERM)
	go watchSignals(sigs, config)

	// wait until the Done channel is terminated before cleanly exiting.
	<-config.Done
	// make sure that the statsdone channel has closed gracefully
	<-config.statsDone
	// make sure the powerdns worker has closed gracefully.
	<-config.pdnsDone
}
