package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// pdnsClient stores the configuration for the powerdns client.
type pdnsClient struct {
	Client *http.Client
	Host   string
	APIKey string

	versionOnce         sync.Once
	serverVersion       pdnsVersion
	serverVersionParsed bool
	usePrometheus       bool

	legacyPath     string
	prometheusPath string
}

type pdnsVersion struct {
	major int
	minor int
	patch int
}

// pdnsStat incoming statistics type
type pdnsStat struct {
	Size  interface{} `json:"size"`
	Value interface{} `json:"value"`
	Name  string      `json:"name"`
	Type  string      `json:"type"`
}

// Initialise a new powerdns client.
//
//nolint:mnd // maxIdleConns is not a mnd
func (pdns *pdnsClient) Initialise(config *Config) {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    time.Duration(delayMultipler) * *config.interval,
		DisableCompression: true,
	}

	baseURL := fmt.Sprintf("http://%s", net.JoinHostPort(*config.pdnsHost, *config.pdnsPort))
	pdns.legacyPath = fmt.Sprintf("%s/api/v1/servers/localhost/statistics", baseURL)
	pdns.prometheusPath = fmt.Sprintf("%s/metrics", baseURL)
	// Default to legacy until we discover the server version.
	pdns.Host = pdns.legacyPath

	pdns.APIKey = *config.pdnsAPIKey
	// Ensure polls don't hang indefinitely. Default to 10s, but for very small intervals
	// keep the timeout below the poll cadence.
	timeout := 10 * time.Second
	if config.interval != nil && *config.interval > 0 {
		candidate := *config.interval / 2
		if candidate > 0 && candidate < timeout {
			timeout = candidate
		}
	}
	// Avoid a near-zero timeout making the service unusable.
	if timeout < 100*time.Millisecond {
		timeout = 100 * time.Millisecond
	}
	pdns.Client = &http.Client{Transport: transport, Timeout: timeout}
}

// Worker wraps a ticker for task execution to query the powerdns API.
func (pdns *pdnsClient) Worker(config *Config) {
	log.Info("Starting PowerDNS statistics worker...")
	interval := time.NewTicker(*config.interval)
	defer interval.Stop()
	defer close(config.pdnsExited)
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
			if pdns.usePrometheus {
				err = decodePrometheusStats(response, config)
			} else {
				err = decodeStats(response, config)
			}
			if err != nil {
				log.Error("powerdns decodeStats",
					zap.Error(err),
				)
			}
		case <-config.stop:
			log.Info("exiting from pdns Worker.")
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

	ensurePrometheusMode := func(resp *http.Response) {
		pdns.versionOnce.Do(func() {
			version := resp.Header.Get("Server")
			if v, ok := parsePDNSServerHeader(version); ok {
				pdns.serverVersion = v
				pdns.serverVersionParsed = true
				pdns.usePrometheus = isAtLeast(v, pdnsVersion{major: 4, minor: 3, patch: 0})
				if pdns.usePrometheus {
					pdns.Host = pdns.prometheusPath
				} else {
					pdns.Host = pdns.legacyPath
				}
			}
		})
	}

	// If this is the first poll, hit the legacy endpoint to discover the server version,
	// then switch to /metrics for >=4.3.
	if !pdns.serverVersionParsed {
		resp, err := pdns.doRequest(pdns.legacyPath)
		if err != nil {
			return &http.Response{}, err
		}
		ensurePrometheusMode(resp)
		if pdns.usePrometheus {
			resp.Body.Close()
			return pdns.doRequest(pdns.prometheusPath)
		}
		return resp, nil
	}

	requestURL := pdns.legacyPath
	if pdns.usePrometheus {
		requestURL = pdns.prometheusPath
	}
	request, err := http.NewRequest("GET", requestURL, http.NoBody)
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
		response.Body.Close()
		return &http.Response{}, fmt.Errorf("expected status_code %d got %d returned from PowerDNS", http.StatusOK, response.StatusCode)
	}

	return response, nil
}

func (pdns *pdnsClient) doRequest(url string) (*http.Response, error) {
	request, err := http.NewRequest("GET", url, http.NoBody)
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
		response.Body.Close()
		return &http.Response{}, fmt.Errorf("expected status_code %d got %d returned from PowerDNS", http.StatusOK, response.StatusCode)
	}
	return response, nil
}

func parsePDNSServerHeader(server string) (pdnsVersion, bool) {
	idx := strings.Index(server, "PowerDNS/")
	if idx < 0 {
		return pdnsVersion{}, false
	}
	ver := strings.TrimPrefix(server[idx:], "PowerDNS/")
	ver = strings.TrimSpace(ver)
	if ver == "" {
		return pdnsVersion{}, false
	}
	parts := strings.SplitN(ver, ".", 4)
	if len(parts) < 2 {
		return pdnsVersion{}, false
	}

	major, err := strconv.Atoi(readNumericPrefix(parts[0]))
	if err != nil {
		return pdnsVersion{}, false
	}
	minor, err := strconv.Atoi(readNumericPrefix(parts[1]))
	if err != nil {
		return pdnsVersion{}, false
	}
	patch := 0
	if len(parts) >= 3 {
		p, err := strconv.Atoi(readNumericPrefix(parts[2]))
		if err == nil {
			patch = p
		}
	}

	return pdnsVersion{major: major, minor: minor, patch: patch}, true
}

func readNumericPrefix(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s[:i]
		}
	}
	return s
}

func isAtLeast(got pdnsVersion, want pdnsVersion) bool {
	if got.major != want.major {
		return got.major > want.major
	}
	if got.minor != want.minor {
		return got.minor > want.minor
	}
	return got.patch >= want.patch
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

	for i := 0; i < len(stats); i++ {
		switch stats[i].Type {
		case "StatisticItem":
			err = decodeStringStat(response, stats[i], config, false)
			if err != nil {
				return err
			}
		case "MapStatisticItem": // adds the new MapStatisticsItem type added in 4.2.0
			arr, ok := stats[i].Value.([]interface{})
			if !ok {
				continue
			}
			for _, stat := range arr {
				m, ok := stat.(map[string]interface{})
				if !ok {
					continue
				}

				name, ok := m["name"].(string)
				if !ok || name == "" {
					continue
				}

				valRaw, ok := m["value"]
				if !ok {
					continue
				}

				var val int64
				switch v := valRaw.(type) {
				case string:
					parsed, err := strconv.ParseInt(v, 10, 64)
					if err != nil {
						continue
					}
					val = parsed
				case float64:
					val = int64(v)
				default:
					continue
				}

				n := fmt.Sprintf("%s-%s", stats[i].Name, name)
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
		case "RingStatisticItem":
			if str, ok := stats[i].Size.(string); ok {
				val, err := strconv.ParseInt(str, 10, 64)
				if err != nil {
					return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
				}
				// populate the map with metrics names.
				if _, ok := config.counterCumulativeValues[stats[i].Name]; !ok {
					config.counterCumulativeValues[stats[i].Name] = -1
				}
				config.StatsChan <- Statistic{
					Name:  stats[i].Name,
					Type:  gauge,
					Value: val,
				}
			}
		default:
			// this allows for forward compatibility of powerdns adds new metrics types we just skip over them.
			// we emit a metric so that we know this is happening. powerdns.(recursor|authoritative).unknown.(type)
			err := decodeStringStat(response, stats[i], config, true)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func decodePrometheusStats(response *http.Response, config *Config) error {
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	metricTypes := make(map[string]string)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "# TYPE ") {
				fields := strings.Fields(line)
				// # TYPE <metric_name> <counter|gauge|...>
				if len(fields) >= 4 {
					metricTypes[fields[2]] = fields[3]
				}
			}
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		nameRaw := fields[0]
		name := nameRaw
		if idx := strings.IndexByte(nameRaw, '{'); idx >= 0 {
			name = nameRaw[:idx]
		}
		if name == "" {
			continue
		}

		f, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		val := int64(f)

		t := metricTypes[name]
		statType := gauge
		if t == "counter" {
			statType = counterCumulative
		}

		if statType == counterCumulative {
			if _, ok := config.counterCumulativeValues[name]; !ok {
				config.counterCumulativeValues[name] = -1
			}
		}

		config.StatsChan <- Statistic{Name: name, Type: statType, Value: val}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func decodeStringStat(response *http.Response, stat pdnsStat, config *Config, unknown bool) error {
	var n string
	if str, ok := stat.Value.(string); ok {
		val, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return fmt.Errorf("unable to convert %s value string to int64 in decodeStats()", str)
		}
		switch unknown {
		case true:
			n = fmt.Sprintf("unknown.%s", stat.Type)
		default:
			n = stat.Name
		}

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
	}
	return nil
}
