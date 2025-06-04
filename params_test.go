//go:build unit_test

package main

import (
	"net/url"
	"reflect"
	"testing"
)

func Test_merge(t *testing.T) {
	type args struct {
		orig    url.Values
		newVals url.Values
	}
	tests := []struct {
		name    string
		args    args
		want    url.Values
		wantErr bool
	}{
		{
			name: "simple",
			args: args{
				orig: url.Values{
					"foo":  []string{"bar"},
					"whiz": []string{"bang"},
				},

				newVals: map[string][]string{
					"whiz": {"test"},
				},
			},
			want: url.Values{
				"foo":  []string{"bar"},
				"whiz": []string{"test"},
			},
			wantErr: false,
		},
		{
			name: "empty orig",
			args: args{
				orig: url.Values{},
				newVals: map[string][]string{
					"whiz": {"test"},
				},
			},
			want: url.Values{
				"whiz": []string{"test"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := combine(tt.args.orig, tt.args.newVals)
			if (err != nil) != tt.wantErr {
				t.Errorf("combine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("combine() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_replace(t *testing.T) {
	type args struct {
		newVals map[string][]string
	}
	tests := []struct {
		name    string
		args    args
		want    *url.Values
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := replace(tt.args.newVals)
			if (err != nil) != tt.wantErr {
				t.Errorf("replace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("replace() got = %v, want %v", got, tt.want)
			}
		})
	}
}
