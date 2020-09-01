package main

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

func testConfig() *Config {
	return &Config{
		statsHost:  stringPtr("127.0.0.1"),
		statsPort:  stringPtr("8199"),
		interval:   timePtr(time.Duration(1) * time.Second),
		pdnsHost:   stringPtr("127.0.0.1"),
		pdnsPort:   stringPtr("8089"),
		pdnsAPIKey: stringPtr("x-api-key"),
		recursor:   boolPtr(true),
	}
}

func TestDNSWorker(t *testing.T) {
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DNSWorker(tt.args.config, tt.args.c)
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
		wantErr bool
	}{
		{
			name: "recursor 4.0 valid",
			args: args{
				response: &http.Response{
					Body: ioutil.NopCloser(strings.NewReader("hello world")),
				},
				config: &Config{},
			},
			wantErr: false,
		},
		{
			name: "recursor 4.1 valid",
			args: args{
				response: &http.Response{
					Body: ioutil.NopCloser(strings.NewReader("hello world")),
				},
				config: &Config{},
			},
			wantErr: false,
		},
		{
			name: "recursor 4.2 valid",
			args: args{
				response: &http.Response{
					Body: ioutil.NopCloser(strings.NewReader("hello world")),
				},
				config: &Config{},
			},
			wantErr: false,
		},
		{
			name: "recursor 4.3 valid",
			args: args{
				response: &http.Response{
					Body: ioutil.NopCloser(strings.NewReader("hello world")),
				},
				config: &Config{},
			},
			wantErr: false,
		},
		{
			name: "recursor 4.4 valid",
			args: args{
				response: &http.Response{
					Body: ioutil.NopCloser(strings.NewReader("hello world")),
				},
				config: &Config{},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := decodeStats(tt.args.response, tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("decodeStats() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
