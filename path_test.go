//go:build unit_test

package main

import (
	"regexp"
	"testing"
)

func Test_rewrite(t *testing.T) {
	type args struct {
		path string
		from string
		to   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "simple capture",
			args: args{
				path: "example.com/test",
				from: "example.com/(?<CAPTURE>.+)",
				to:   "https://foo.com/bar/$CAPTURE",
			},
			want: "/bar/test",
		},
		{
			name: "long capture",
			args: args{
				path: "example.com/test/hello/world",
				from: "example.com/(?<CAPTURE>.+)",
				to:   "https://foo.com/bar/$CAPTURE",
			},
			want: "/bar/test/hello/world",
		},
		{
			name: "multi capture group",
			args: args{
				path: "example.com/test/hello/world",
				from: `example.com/test/(?<CAPTURE>\w+)/(?<GROUP2>\w+)`,
				to:   "https://foo.com/bar/$GROUP2/$CAPTURE",
			},
			want: "/bar/world/hello",
		},
		{
			name: "no regex",
			args: args{
				path: "example.com/test/hello/world",
				from: `example.com/test/hello/world`,
				to:   "https://foo.com/",
			},
			want: "/",
		},
		{
			name: "unused capture",
			args: args{
				path: "example.com/test/hello/world",
				from: `example.com/test/(?<CAPTURE>\w+)/(?<GROUP2>\w+)`,
				to:   "https://foo.com/xyz",
			},
			want: "/xyz",
		},
		{
			name: "unused capture",
			args: args{
				path: "example.com/test/hello/world/xyz",
				from: `example.com/test/(?<CAPTURE>\w+)/(?<GROUP2>\w+)/xyz`,
				to:   "https://foo.com/xyz",
			},
			want: "/xyz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp, err := regexp.Compile(tt.args.from)
			if err != nil {
				t.Fatalf("failed to compile regexp: %v", err)
			}
			if got, _ := rewritePath(tt.args.path, exp, tt.args.to); got != tt.want {
				t.Errorf("got = '%v', want '%v'", got, tt.want)
			}
		})
	}
}
