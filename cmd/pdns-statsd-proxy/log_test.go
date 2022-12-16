package main

import (
	"testing"

	"go.uber.org/zap"
)

func TestLoggerPrintln(t *testing.T) {
	log = zap.NewExample(zap.AddCaller(), zap.WithCaller(true)).Named(provider)
	type args struct {
		v []interface{}
	}
	tests := []struct {
		name string
		l    *logger
		args args
	}{
		{
			name: "testing logger",
			l:    &logger{},
			args: args{
				v: []interface{}{"foo"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &logger{}
			l.Println(tt.args.v...)
		})
	}
}

func TestInitLogger(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "testing init",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := initLogger(); (err != nil) != tt.wantErr {
				t.Errorf("initLogger() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
