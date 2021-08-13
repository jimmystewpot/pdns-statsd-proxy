package main

import (
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/quipo/statsd"
)

func TestGaugeMetrics(t *testing.T) {
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
	// setup a backgroun udp listener to accept the statsd datagrams/packets.
	go func(config *Config) {
		udpAddr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(*config.statsHost, *config.statsPort))
		if err != nil {
			t.Error(err)
		}
		statsSrv, err := net.ListenUDP("udp4", udpAddr)
		if err != nil {
			t.Error(err)
		}
		defer statsSrv.Close()
		for {
			select {
			case <-config.done:
				return
			default:
				buf := make([]byte, statsd.UDPPayloadSize)
				_, _, err := statsSrv.ReadFromUDP(buf)
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
				for r := 0; r <= 3; r++ {
					responseBody := &http.Response{
						Body: ioutil.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3"))),
					}
					err := decodeStats(responseBody, config)
					if err != nil {
						t.Error(err)
					}
					responseBody.Body.Close()
				}
				wg.Done()
			}(tt.args.config, &wg)

			// wait until the statistics have been sent.
			wg.Wait()
			go statsWorker(tt.args.config)

			time.Sleep(time.Duration(3500) * time.Millisecond)
			tt.args.config.statsDone <- true
			tt.args.config.done <- true // close the udp listener.
		})
	}
}

func TestNewStatsClient(t *testing.T) {
	type args struct {
		config *Config
	}
	tests := []struct {
		name      string
		args      args
		statsHost string
		wantErr   bool
	}{
		{
			name: "Good Configuration",
			args: args{
				config: testConfig(),
			},
			statsHost: "127.0.0.1",
			wantErr:   false,
		},
		{
			name: "Bad Configuration - invalid host",
			args: args{
				config: testConfig(),
			},
			statsHost: "",
			wantErr:   true,
		},
		{
			name: "Bad Configuration - invalid ip",
			args: args{
				config: testConfig(),
			},
			statsHost: "a.b.c.d",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.config.statsHost = stringPtr(tt.statsHost)
			_, err := NewStatsClient(tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStatsClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
