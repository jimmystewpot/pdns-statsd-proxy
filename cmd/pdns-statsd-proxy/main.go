package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quipo/statsd"
	"go.uber.org/zap"
)

var (
	log   = zap.NewExample().Named("pdns-statistics-proxy")
	stats *statsd.StatsdBuffer
	gauge = gaugeMetrics()
	rates = rateMetrics()
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

	config.statsHost = flag.String("statsd", "127.0.0.1", "The statsd server to emit metrics")
	config.statsPort = flag.Int("statsdport", 8125, "The port that statsd is listening on")
	config.recursor = flag.Bool("recursor", true, "Query recursor statistics")
	config.pdnsHost = flag.String("pdnsHost", "127.0.0.1", "The host to query for powerdns statistics")
	config.pdnsAPIKey = flag.String("key", "", "The api key for the powerdns api")
	interval := flag.Int("interval", 15, "The interval to emit metrics in seconds")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	config.StatsChan = make(chan Statistic, 1000)
	config.Done = make(chan bool, 1)
	sigs := make(chan os.Signal, 1)

	if !validateConfiguration(config, interval) {
		log.Fatal("Unable to process configuration, missing flags")
	}

	// initiate the statsd client.
	var err error
	stats, err = NewStatsClient(config)
	if err != nil {
		log.Fatal("Unable to initiate statsd client")
	}

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
