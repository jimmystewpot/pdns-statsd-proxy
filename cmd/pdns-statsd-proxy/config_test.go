package main

import "time"

func testConfig() *Config {
	return &Config{
		statsHost:  stringPtr("127.0.0.1"),
		statsPort:  stringPtr("8199"),
		interval:   timePtr(time.Duration(1) * time.Second),
		pdnsHost:   stringPtr("127.0.0.1"),
		pdnsPort:   stringPtr("8089"),
		pdnsAPIKey: stringPtr("x-api-key"),
		recursor:   boolPtr(true),
		StatsChan:  make(chan Statistic, 1000),
		Done:       make(chan bool, 1),
	}
}
