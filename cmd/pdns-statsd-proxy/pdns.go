package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// DNSClient ...
type DNSClient struct {
	Host   string
	APIKey string
	C      *http.Client
}

// PDNSStat incoming statistics type
type PDNSStat struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// NewPdnsClient returns a powerdns client.
func NewPdnsClient(config *Config) *DNSClient {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    *config.interval,
		DisableCompression: true,
	}
	host := fmt.Sprintf("http://%s:8080/api/v1/servers/localhost/statistics", *config.pdnsHost)
	return &DNSClient{
		Host:   host,
		APIKey: *config.pdnsAPIKey,
		C:      &http.Client{Transport: transport},
	}
}

// DNSWorker wraps a ticker for task execution.
func DNSWorker(config *Config, c *DNSClient) {
	log.Info("Starting PowerDNS statistics worker...")
	interval := time.NewTicker(*config.interval)
	for {
		select {
		case <-interval.C:
			c.Poll(config)
		case <-config.Done:
			log.Warn("done closed, exiting from DNSWorker.")
			return
		}
	}
}

// Poll for statistics
func (c *DNSClient) Poll(config *Config) {
	request, err := http.NewRequest("GET", c.Host, nil)
	if err != nil {
		log.Fatal("error in powerdns client request",
			zap.Error(err),
		)
	}
	request.Header.Add("X-API-Key", c.APIKey)
	response, err := c.C.Do(request)
	if err != nil {
		log.Fatal("error in powerdns client request",
			zap.Int("status_code", response.StatusCode),
			zap.Error(err),
		)
	}
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal("error reading response body",
			zap.Error(err),
		)
	}

	tmp := []PDNSStat{}
	err = json.Unmarshal(body, &tmp)
	if err != nil {
		log.Warn("unable to unmarshal json from powerdns Poll()",
			zap.Error(err),
		)
		return
	}

	log.Info("successfully fetched PowerDNS statistics")
	for _, stat := range tmp {
		val, err := strconv.ParseInt(stat.Value, 10, 64)
		if err != nil {
			log.Warn("unable to convert value string to int64 in Poll()")
			continue
		}
		if _, ok := rates[stat.Name]; ok {
			config.StatsChan <- Statistic{
				Name:  stat.Name,
				Type:  "rate",
				Value: val,
			}
		}
		if _, ok := gauge[stat.Name]; ok {
			config.StatsChan <- Statistic{
				Name:  stat.Name,
				Type:  "gauge",
				Value: val,
			}
		}
	}

	return
}
