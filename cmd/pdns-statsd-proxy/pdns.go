package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// pdnsClient stores the configuration for the powerdns client.
type pdnsClient struct {
	Host   string
	APIKey string
	Client *http.Client
}

// pdnsStat incoming statistics type
type pdnsStat struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Size  interface{} `json:"size"`
	Value interface{} `json:"value"`
}

// Initialise a new powerdns client.
func (pdns *pdnsClient) Initialise(config *Config) error {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    *config.interval * 4,
		DisableCompression: true,
	}

	hostPort := net.JoinHostPort(*config.pdnsHost, *config.pdnsPort)

	pdns.Host = fmt.Sprintf("http://%s/api/v1/servers/localhost/statistics", hostPort)

	pdns.APIKey = *config.pdnsAPIKey
	pdns.Client = &http.Client{Transport: transport}

	return nil
}

// Worker wraps a ticker for task execution to query the powerdns API.
func (pdns *pdnsClient) Worker(config *Config) {
	log.Info("Starting PowerDNS statistics worker...")
	interval := time.NewTicker(*config.interval)
	for {
		select {
		case <-interval.C:
			response, err := pdns.Poll()
			if err != nil {
				log.Warn("powerdns client",
					zap.Error(err),
				)
				continue
			}
			err = decodeStats(response, config)
			if err != nil {
				log.Warn("powerdns decodeStats",
					zap.Error(err),
				)
			}
		case <-config.pdnsDone:
			log.Warn("exiting from pdns Worker.")
			close(config.pdnsDone)
			return
		}
	}
}

// Poll for statistics
func (pdns *pdnsClient) Poll() (*http.Response, error) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				log.Warn("recovered from panic in pdnsClient.Polll()",
					zap.Error(err),
				)
			}
		}
	}()

	request, err := http.NewRequest("GET", pdns.Host, nil)
	if err != nil {
		return &http.Response{}, fmt.Errorf("unable to instantiate new http client: %s", err)
	}

	request.Header.Add("X-API-Key", pdns.APIKey)
	request.Header.Add("User-Agent", provider)

	response, err := pdns.Client.Do(request)
	if err != nil {
		return &http.Response{}, err
	}

	if response.StatusCode != http.StatusOK {
		return &http.Response{}, fmt.Errorf(fmt.Sprintf("expected status_code %d got %d returned from PowerDNS", http.StatusOK, response.StatusCode))
	}

	log.Info("successfully queried PowerDNS statistics")

	return response, nil
}

func decodeStats(response *http.Response, config *Config) error {
	defer response.Body.Close()

	stats := make([]pdnsStat, 0)

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &stats)
	if err != nil {
		return err
	}

	for _, stat := range stats {
		switch stat.Type {
		case "StatisticItem":
			if str, ok := stat.Value.(string); ok {
				val, err := strconv.ParseInt(str, 10, 64)
				if err != nil {
					return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
				}
				if _, ok := gaugeNames[stat.Name]; ok {
					config.StatsChan <- Statistic{
						Name:  stat.Name,
						Type:  gauge,
						Value: val,
					}
					continue
				}

				// populate the map with metrics names.
				if _, ok := counterCumulativeValues[stat.Name]; !ok {
					counterCumulativeValues[stat.Name] = -1
				}

				config.StatsChan <- Statistic{
					Name:  stat.Name,
					Type:  counterCumulative,
					Value: val,
				}
			}
		case "MapStatisticItem": // adds the new MapStatisticsItem type added in 4.2.0

			for _, i := range stat.Value.([]interface{}) {

				if m, ok := i.(map[string]interface{}); ok {
					val, err := strconv.ParseInt(m["value"].(string), 10, 64)
					if err != nil {
						return fmt.Errorf("unable to convert %s string to int64 in decodeStats()", m["value"])
					}
					n := fmt.Sprintf("%s-%s", stat.Name, m["name"])
					// populate the map with metrics names.
					if _, ok := counterCumulativeValues[n]; !ok {
						counterCumulativeValues[n] = -1
					}
					config.StatsChan <- Statistic{
						Name:  n,
						Type:  counterCumulative,
						Value: val,
					}
				}
			}
		case "RingStatisticItem":
			if str, ok := stat.Size.(string); ok {
				val, err := strconv.ParseInt(str, 10, 64)
				if err != nil {
					return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
				}
				n := fmt.Sprintf("%s", stat.Name)
				// populate the map with metrics names.
				if _, ok := counterCumulativeValues[n]; !ok {
					counterCumulativeValues[n] = -1
				}
				config.StatsChan <- Statistic{
					Name:  fmt.Sprintf(stat.Name),
					Type:  gauge,
					Value: val,
				}
			}
		default:
			continue
		}
	}
	return nil
}
