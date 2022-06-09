package main

import (
	"encoding/json"
	"fmt"
	"io"
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
func (pdns *pdnsClient) Initialise(config *Config) {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    *config.interval * 4,
		DisableCompression: true,
	}

	hostPort := net.JoinHostPort(*config.pdnsHost, *config.pdnsPort)

	pdns.Host = fmt.Sprintf("http://%s/api/v1/servers/localhost/statistics", hostPort)

	pdns.APIKey = *config.pdnsAPIKey
	pdns.Client = &http.Client{Transport: transport}
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
				log.Error("powerdns client",
					zap.Error(err),
				)
				continue
			}
			err = decodeStats(response, config)
			if err != nil {
				log.Error("powerdns decodeStats",
					zap.Error(err),
				)
			}
		case <-config.pdnsDone:
			log.Info("exiting from pdns Worker.")
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
				log.Error("recovered from panic in pdnsClient.Polll()",
					zap.Error(err),
				)
			}
		}
	}()

	request, err := http.NewRequest("GET", pdns.Host, http.NoBody)
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
		return &http.Response{}, fmt.Errorf("expected status_code %d got %d returned from PowerDNS", http.StatusOK, response.StatusCode)
	}

	log.Info("successfully queried PowerDNS statistics")

	return response, nil
}

func decodeStats(response *http.Response, config *Config) error {
	defer response.Body.Close()

	stats := make([]pdnsStat, 0)

	body, err := io.ReadAll(response.Body)
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
			err := statisticItem(stat, config)
			if err != nil {
				return err
			}
			continue
		case "MapStatisticItem": // adds the new MapStatisticsItem type added in 4.2.0
			err := mapStatisticItem(stat, config)
			if err != nil {
				return err
			}
			continue
		case "RingStatisticItem":
			err := ringStatisticItem(stat, config)
			if err != nil {
				return err
			}
			continue
		default:
			// this allows for forward compatibility of powerdns adds new metrics types we just skip over them.
			// we emit a metric so that we know this is happening. powerdns.(recursor|authoritative).unknown.(type)
			if str, ok := stat.Value.(string); ok {
				val, err := strconv.ParseInt(str, base10, bitSize64)
				if err != nil {
					return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
				}
				n := fmt.Sprintf("unknown.%s", stat.Type)
				// populate the map with metrics names.
				if _, ok := config.counterCumulativeValues[n]; !ok {
					config.counterCumulativeValues[n] = -1
				}

				config.StatsChan <- Statistic{
					Name:  n,
					Type:  counterCumulative,
					Value: val,
				}
				version := response.Header.Get("Server")
				log.Info("unknown metric type in api response",
					zap.String("pdns_version", version),
					zap.String("type", stat.Type),
					zap.String("name", stat.Name),
					zap.Int64("value", val),
				)
				continue
			}
		}
	}
	return nil
}

// statisticItem emits a statistic for basic metric types.
func statisticItem(stat pdnsStat, config *Config) error {
	if str, ok := stat.Value.(string); ok {
		val, err := strconv.ParseInt(str, base10, bitSize64)
		if err != nil {
			return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
		}
		if _, ok := gaugeNames[stat.Name]; ok {
			config.StatsChan <- Statistic{
				Name:  stat.Name,
				Type:  gauge,
				Value: val,
			}
			return nil
		}

		// populate the map with metrics names.
		if _, ok := config.counterCumulativeValues[stat.Name]; !ok {
			config.counterCumulativeValues[stat.Name] = -1
		}

		config.StatsChan <- Statistic{
			Name:  stat.Name,
			Type:  counterCumulative,
			Value: val,
		}
	}
	return nil
}

//nolint
func mapStatisticItem(stat pdnsStat, config *Config) error {
	for _, i := range stat.Value.([]interface{}) {
		if m, ok := i.(map[string]interface{}); ok {
			val, err := strconv.ParseInt(m["value"].(string), base10, bitSize64)
			if err != nil {
				return fmt.Errorf("unable to convert %s string to int64 in decodeStats()", m["value"])
			}
			n := fmt.Sprintf("%s-%s", stat.Name, m["name"])
			// populate the map with metrics names.
			if _, ok := config.counterCumulativeValues[n]; !ok {
				config.counterCumulativeValues[n] = -1
			}
			config.StatsChan <- Statistic{
				Name:  n,
				Type:  counterCumulative,
				Value: val,
			}
		}
	}
	return nil
}

func ringStatisticItem(stat pdnsStat, config *Config) error {
	if str, ok := stat.Size.(string); ok {
		val, err := strconv.ParseInt(str, base10, bitSize64)
		if err != nil {
			return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
		}

		// populate the map with metrics names.
		if _, ok := config.counterCumulativeValues[stat.Name]; !ok {
			config.counterCumulativeValues[stat.Name] = -1
		}
		config.StatsChan <- Statistic{
			Name:  fmt.Sprintf(stat.Name),
			Type:  gauge,
			Value: val,
		}
	}
	return nil
}
