package main

import (
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func Test_gaugeMetrics(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{
			name:    "valid gauge",
			want:    "uptime",
			wantErr: false,
		},
		{
			name:    "invalid gauge",
			want:    "xxxx",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gaugeMetrics()
			if _, ok := got[tt.want]; !ok {
				if !tt.wantErr {
					t.Errorf("gaugeMetrics() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestStatsWorker(t *testing.T) {
	var wg sync.WaitGroup

	type args struct {
		config *Config
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "stats worker start",
			args: args{
				config: testConfig(),
			},
		},
	}
	go func(config *Config) {
		statsSrv, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", *config.statsPort))
		if err != nil {
			t.Error(err)
		}
		defer statsSrv.Close()
		for {
			select {
			case <-config.Done:
				return
			default:
				buf := make([]byte, 4096)
				_, _, err := statsSrv.ReadFrom(buf)
				if err != nil {
					continue
				}
			}
		}
	}(testConfig())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, _ = NewStatsClient(tt.args.config)
			// this function sends the metrics down the channel for the worker to process.
			// it needs to have a wait group to wait for the system to prime otherwise it causes
			// a data race.
			wg.Add(1)
			go func(config *Config, wg *sync.WaitGroup) {
				counterCumulativeValues = make(map[string]int64)
				jsonBody := &http.Response{
					Body: ioutil.NopCloser(strings.NewReader(readpdnsTestData("4.3.3"))),
				}
				err := decodeStats(jsonBody, tt.args.config)
				if err != nil {
					t.Error(err)
				}
				wg.Done()
			}(tt.args.config, &wg)

			wg.Wait()
			go StatsWorker(tt.args.config)

			time.Sleep(time.Duration(500) * time.Millisecond)
			close(tt.args.config.Done)
		})
	}
}