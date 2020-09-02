package main

import (
	"os"
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
		StatsChan:  make(chan Statistic, 1000),
		Done:       make(chan bool, 1),
	}
}

func Test_getEnvStr(t *testing.T) {
	type args struct {
		name string
		def  string
	}
	tests := []struct {
		name string
		args args
		want string
		set  bool
	}{
		{
			name: "test environment FOO_BAR",
			args: args{
				name: "FOO_BAR",
				def:  "FOO",
			},
			want: "FOO",
			set:  false,
		},
		{
			name: "test environment BAR_FOO",
			args: args{
				name: "BAR_FOO",
				def:  "",
			},
			want: "DEFAULT",
			set:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				os.Setenv(tt.args.name, tt.want)
			}
			if got := getEnvStr(tt.args.name, tt.args.def); *got != tt.want {
				t.Errorf("getEnvStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_checkpdnsAPIKey(t *testing.T) {
	type args struct {
		config *Config
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Valid API Key",
			args: args{
				config: testConfig(),
			},
			want: true,
		},
		{
			name: "No API Key",
			args: args{
				config: &Config{
					pdnsAPIKey: stringPtr(""),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkpdnsAPIKey(tt.args.config); got != tt.want {
				t.Errorf("checkpdnsAPIKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_checkStatsHost(t *testing.T) {
	type args struct {
		config *Config
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Valid Statsd Host",
			args: args{
				config: testConfig(),
			},
			want: true,
		},
		{
			name: "No Statsd host",
			args: args{
				config: &Config{
					statsHost: stringPtr(""),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkStatsHost(tt.args.config); got != tt.want {
				t.Errorf("checkStatsHost() = %v, want %v", got, tt.want)
			}
		})
	}
}
