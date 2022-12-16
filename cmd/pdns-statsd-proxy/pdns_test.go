package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func readpdnsTestData(version string) string {
	vers := strings.ReplaceAll(version, ".", "_")
	jsonFile := fmt.Sprintf("pdns_response_test_data/%s.json", vers)
	f, _ := os.ReadFile(jsonFile)

	return string(f)
}

func testDNSClient(config *Config) *pdnsClient {
	// initiate the powerdns client.
	pdnsClient := new(pdnsClient)
	pdnsClient.Initialise(config)
	return pdnsClient
}

func testHeader() http.Header {
	header := make(http.Header)
	header.Set("Server", "PowerDNS/4.9.0")
	return header
}

func TestDecodeStats(t *testing.T) {
	type args struct {
		response *http.Response
		config   *Config
	}
	tests := []struct {
		name     string
		args     args
		count    int
		recursor bool
		wantErr  bool
	}{
		{
			name: "recursor 4.3 valid",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    114,
			recursor: true,
			wantErr:  false,
		},
		{
			name: "recursor 4.3 invalid",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3-bad"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    114,
			recursor: true,
			wantErr:  true,
		},
		{
			name: "auth 4.3 valid",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("auth-4.3.0"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    86,
			recursor: false,
			wantErr:  false,
		},
		{
			name: "auth 4.3 invalid",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("auth-4.3.0-bad"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    78,
			recursor: false,
			wantErr:  true,
		},
		{
			name: "recursor unknown metric type",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("recursor-unknown"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			count:    115,
			recursor: true,
			wantErr:  false,
		},
		{
			name: "recursor map type invalid int",
			args: args{
				response: &http.Response{
					Body:   io.NopCloser(strings.NewReader(readpdnsTestData("recursor-4.3.3-bad_ints"))),
					Header: testHeader(),
				},
				config: testConfig(),
			},
			recursor: true,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.recursor {
				tt.args.config.recursor = ptrBool(tt.recursor)
			}
			if err := decodeStats(tt.args.response, tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("decodeStats() error = %v, wantErr %v", err, tt.wantErr)
			}
			if (len(tt.args.config.StatsChan) != tt.count) != tt.wantErr {
				t.Errorf("expected %d stats got %d", tt.count, len(tt.args.config.StatsChan))
			}
		})
	}
}

func TestPdnsClientWorker(t *testing.T) {
	type args struct {
		config *Config
		err    bool
	}
	tests := []struct {
		name              string
		args              args
		testDataFile      string
		testResponseCode  int
		testAuthoritative bool
	}{
		{
			name: "Good HTTP Response, Good Payload",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-4.3.3",
			testResponseCode: http.StatusOK,
		},
		{
			name: "Good HTTP Response, Bad Payload",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-4.3.3-bad",
			testResponseCode: http.StatusOK,
		},
		{
			name: "Bad HTTP Response, Good Payload",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-4.3.3",
			testResponseCode: http.StatusUnauthorized,
		},
		{
			name: "Bad HTTP Response, Bad Payload",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-4.3.3-bad",
			testResponseCode: http.StatusUnauthorized,
		},
		{
			name: "Good HTTP response, Good Payload Unknown Entry",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-unknown",
			testResponseCode: http.StatusOK,
		},
		{
			name: "Good HTTP response, Bad Payload Unknown Entry",
			args: args{
				config: testConfig(),
			},
			testDataFile:     "recursor-unknown-bad",
			testResponseCode: http.StatusOK,
		},
		{
			name: "Good HTTP response, Bad Server",
			args: args{
				config: testConfig(),
				err:    true,
			},
			testDataFile:     "recursor-unknown-bad",
			testResponseCode: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdns := testDNSClient(tt.args.config)

			// setup a local http mock to simulate the powerdns api
			var listener net.Listener
			var err error
			switch tt.args.err {
			case true:
				listener, err = net.Listen("tcp", net.JoinHostPort(*tt.args.config.pdnsHost, "5555"))
			case false:
				listener, err = net.Listen("tcp", net.JoinHostPort(*tt.args.config.pdnsHost, *tt.args.config.pdnsPort))
			}
			if err != nil {
				t.Errorf("got error trying to start mock http listener: %s", err)
			}

			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.testResponseCode)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Server", "PowerDNS/4.0.0-test")
				fmt.Fprint(w, readpdnsTestData(tt.testDataFile))
			}))
			srv.Listener = listener
			srv.Start()

			// close the channel in the background to test a correct exit state.
			go func(config *Config) {
				time.Sleep(time.Duration(1000) * time.Millisecond)
				config.pdnsDone <- true
			}(tt.args.config)

			go pdns.Worker(tt.args.config)
			time.Sleep(time.Duration(1500) * time.Millisecond)

			// close the mock server.
			srv.Close()
		})
	}
}

func Test_pdnsClient_Poll(t *testing.T) {
	type fields struct {
		Host   string
		APIKey string
		Client *http.Client
	}
	tests := []struct {
		name    string
		fields  fields
		want    *http.Response
		wantErr bool
	}{
		{
			name: "Known bad configuration",
			fields: fields{
				Host:   "\1232\\1\\11",
				APIKey: "bad-entry",
				Client: &http.Client{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdns := &pdnsClient{
				Host:   tt.fields.Host,
				APIKey: tt.fields.APIKey,
				Client: tt.fields.Client,
			}
			//nolint:bodyclose // this is testing the error handling working Polls are handled elsewhere in tests.
			_, err := pdns.Poll()
			if (err != nil) != tt.wantErr {
				t.Errorf("pdnsClient.Poll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func ptrBool(b bool) *bool {
	return &b
}
