package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
)

// Config holds all the configuration required to start the service.
type Config struct {
	statsHost               *string
	statsPort               *string
	interval                *time.Duration
	pdnsHost                *string
	pdnsPort                *string
	pdnsAPIKey              *string
	recursor                *bool
	counterCumulativeValues map[string]int64
	StatsChan               chan Statistic
	done                    chan bool // close global
	pdnsDone                chan bool // close the pdns worker
	statsDone               chan bool // close the stats worker
}

func (c *Config) flags() bool {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] \n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	c.statsHost = statsHost
	c.statsPort = statsPort
	c.pdnsHost = pdnsHost
	c.pdnsPort = pdnsPort
	c.pdnsAPIKey = pdnsAPIKey
	c.recursor = recursor
	c.interval = interval

	return flag.Parsed()
}

// Validate the configuration is correct before starting the service.
func (c *Config) Validate() bool {
	if !c.flags() {
		return false
	}
	c.StatsChan = make(chan Statistic, buffer)
	c.done = make(chan bool, 1)
	c.pdnsDone = make(chan bool, 1)
	c.statsDone = make(chan bool, 1)

	statsHost, err := checkStatsHost(c)

	if !statsHost {
		log.Error("statsHost",
			zap.Error(err),
		)
		return false
	}

	apiKey, err := checkpdnsAPIKey(c)
	if !apiKey {
		log.Error("checkdnsAPIKey",
			zap.Error(err),
		)
		return false
	}

	// configuration is all okay, initialise the maps
	c.counterCumulativeValues = make(map[string]int64)

	return true
}

func checkStatsHost(config *Config) (bool, error) {
	if *config.statsHost == "" {
		return false, fmt.Errorf("unable to find the statsd host to send metrics to")
	}
	return true, nil
}

func checkpdnsAPIKey(config *Config) (bool, error) {
	if *config.pdnsAPIKey == "" {
		// check if its in the environment variables list.
		config.pdnsAPIKey = getEnvStr("PDNS_API_KEY", "")
		// if its still empty we can't start.
		if *config.pdnsAPIKey == "" {
			return false, fmt.Errorf("unable to find PowerDNS API key via flags or environment variable PDNS_API_KEY")
		}
	}
	// the key is not empty we should be able to start.
	return true, nil
}

// getEnvStr looks up an environment variable or returns the default value.
func getEnvStr(name string, def string) *string {
	content, found := os.LookupEnv(name)
	if found {
		return &content
	}
	return &def
}

// timePtr returns a pointer for Time.Duration.
func timePtr(t time.Duration) *time.Duration {
	return &t
}

// stringPtr returns a pointer for an input string
func stringPtr(s string) *string {
	return &s
}

// boolPtr returns a pointer for an input boolean
func boolPtr(b bool) *bool {
	return &b
}
