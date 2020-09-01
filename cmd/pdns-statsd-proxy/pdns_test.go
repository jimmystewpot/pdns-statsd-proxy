package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func readpdnsTestData(version string) string {
	vers := strings.ReplaceAll(version, ".", "_")
	jsonFile := fmt.Sprintf("pdns_response_test_data/%s.json", vers)
	f, _ := ioutil.ReadFile(jsonFile)

	return string(f)
}

func testDNSClient() *DNSClient {
	return NewPdnsClient(testConfig())
}

func TestDNSWorker(t *testing.T) {
	counterCumulativeValues = make(map[string]int64)
	type args struct {
		config *Config
		c      *DNSClient
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "test starting of worker",
			args: args{
				config: testConfig(),
				c:      testDNSClient(),
			},
		},
	}
	// start a mock server.
	listener, _ := net.Listen("tcp", "127.0.0.1:8089")
	pdns := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, readpdnsTestData("4.3.3"))
	}))
	pdns.Listener = listener

	pdns.Start()
	defer pdns.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go func(config *Config) {
				time.Sleep(time.Duration(1500) * time.Millisecond)
				close(config.Done)
			}(tt.args.config)
			go DNSWorker(tt.args.config, tt.args.c)
			time.Sleep(time.Duration(1500) * time.Millisecond)
		})
	}
}

func Test_decodeStats(t *testing.T) {
	type args struct {
		response *http.Response
		config   *Config
	}
	tests := []struct {
		name    string
		args    args
		count   int
		wantErr bool
	}{
		{
			name: "recursor 4.3 valid",
			args: args{
				response: &http.Response{
					Body: ioutil.NopCloser(strings.NewReader(readpdnsTestData("4.3.3"))),
				},
				config: testConfig(),
			},
			count:   114,
			wantErr: false,
		},
		{
			name: "recursor 4.3 invalid",
			args: args{
				response: &http.Response{
					Body: ioutil.NopCloser(strings.NewReader(readpdnsTestData("4.3.3-bad"))),
				},
				config: testConfig(),
			},
			count:   114,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counterCumulativeValues = make(map[string]int64)
			if err := decodeStats(tt.args.response, tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("decodeStats() error = %v, wantErr %v", err, tt.wantErr)
			}
			if (len(tt.args.config.StatsChan) != tt.count) != tt.wantErr {
				t.Errorf("expected %d stats got %d", tt.count, len(tt.args.config.StatsChan))
			}
		})
	}
}
