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

	if c.statsHost == nil {
		c.statsHost = statsHost
	}
	c.statsPort = statsPort
	c.pdnsHost = pdnsHost
	c.pdnsPort = pdnsPort
	// if it's not set via flags or tests, check the environment variable for the API key.
	if c.pdnsAPIKey == nil {
		if *pdnsAPIKey == "" {
			c.pdnsAPIKey = getEnvStr("PDNS_API_KEY", "")
		}
	}
	c.recursor = recursor
	c.interval = interval

	return flag.Parsed()
}

// Validate the configuration is correct before starting the service.
func (c *Config) Validate() bool {
	if !c.flags() {
		return false
	}

	err := c.CheckStatsHost()
	if err != nil {
		log.Error("CheckStatsHost",
			zap.Error(err),
		)
		return false
	}

	err = c.CheckAPIKey()
	if err != nil {
		log.Error("checkdnsAPIKey",
			zap.Error(err),
		)
		return false
	}

	// configuration is all okay, initialise the remaining internals
	c.counterCumulativeValues = make(map[string]int64)

	c.StatsChan = make(chan Statistic, statsBufferSize)
	c.done = make(chan bool, 1)
	c.pdnsDone = make(chan bool, 1)
	c.statsDone = make(chan bool, 1)
	return true
}

func (c *Config) CheckStatsHost() error {
	if *c.statsHost == "" {
		return fmt.Errorf("no statsd host specified in the configuration")
	}
	return nil
}

func (c *Config) CheckAPIKey() error {
	if *c.pdnsAPIKey == "" {
		return fmt.Errorf("unable to find PowerDNS API key via flags or environment variable PDNS_API_KEY")
	}
	// the key is not empty we should be able to start.
	return nil
}

// getEnvStr looks up an environment variable or returns the default value.
func getEnvStr(name string, def string) *string {
	content, found := os.LookupEnv(name)
	if found {
		return &content
	}
	return &def
}
