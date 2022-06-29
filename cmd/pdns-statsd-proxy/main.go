package main

import (
	"flag"
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
	buffer            int    = 100
	base10            int    = 10
	bitSize64         int    = 64
	defaultInterval   int    = 15
	conversionErr     string = "unable to convert %s value string to int64 in decodeStats()"
)

var (
	log        *zap.Logger
	stats      *statsd.StatsdClient
	gaugeNames = gaugeMetrics()

	// flag variables set as globals allows us to test various types of flags without needing to hack around
	// the flags package.
	statsHost  = flag.String("statsHost", "127.0.0.1", "The statsd server to emit metrics")
	statsPort  = flag.String("statsPort", "8125", "The port that statsd is listening on")
	pdnsHost   = flag.String("pdnsHost", "127.0.0.1", "The host to query for powerdns statistics")
	pdnsPort   = flag.String("pdnsPort", "8080", "The port that PowerDNS API is accepting connections")
	pdnsAPIKey = flag.String("key", "", "The api key for the powerdns api")
	recursor   = flag.Bool("recursor", true, "Query recursor statistics")
	interval   = flag.Duration("interval", time.Duration(defaultInterval)*time.Second, "The interval to emit metrics in seconds")
)

// handle a graceful exit so that we do not lose data when we restart the service.
func watchSignals(sig chan os.Signal, config *Config) {
	s := <-sig
	log.Info("Caught signal about to cleanly exit.",
		zap.String("signal", s.String()),
	)
	config.pdnsDone <- true  // close downt he pdns worker first
	config.statsDone <- true // close down the statsd worker
	time.Sleep(*config.interval)
	close(config.done) // unblock the main func for a clean exit.
}

func main() {
	err := initLogger()
	if err != nil {
		log.Fatal("unable to initialise logging",
			zap.Error(err))
	}

	config := new(Config)
	if !config.Validate() {
		log.Fatal("Unable to process configuration, missing flags",
			zap.Error(err))
	}

	// initiate the statsd client.
	stats, err = NewStatsClient(config)
	if err != nil {
		log.Fatal("Unable to initiate statsd client",
			zap.Error(err))
	}

	// initiate the powerdns client.
	pdnsClient := new(pdnsClient)
	pdnsClient.Initialise(config)

	// start background worker goroutines.
	go pdnsClient.Worker(config)
	go statsWorker(config)

	// handle signals correctly.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go watchSignals(sigs, config)

	// wait until the Done channel is terminated before cleanly exiting.
	<-config.done
	// make sure that the statsdone channel has closed gracefully
	<-config.statsDone
	// make sure the powerdns worker has closed gracefully.
	<-config.pdnsDone
}
