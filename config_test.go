//go:build unit_test

package main

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"net/url"
	"regexp"
	"testing"
)

func Test_loadConfig(t *testing.T) {
	logger := newTestLogger()

	type args struct {
		path string
	}
	tests := []struct {
		name            string
		args            args
		wantDefaultMiss string
		wantRuleMapping RuleMapping
		wantErr         bool
	}{
		{
			name: "parameters example",
			args: args{
				path: "./fixtures/config_test.yml",
			},
			wantDefaultMiss: "https://httpbin.org/image/jpeg",
			wantRuleMapping: RuleMapping{
				"example.com": Rules{
					{
						From:               "example.com",
						To:                 "https://foo.com/hello",
						Code:               308,
						compiled:           regexp.MustCompile(`.*`),
						CacheControlMaxAge: 604800,
						Parameters: RuleParameters{
							Strategy: "combine",
							Values: map[string][]string{
								"hello": {"world"},
								"foo":   {"bar"},
								"whiz":  {"bang", "test"},
							},
						},
					},
					{
						From:               "example.com/xyz",
						To:                 "https://foo.com/hello",
						Code:               301,
						compiled:           regexp.MustCompile(""),
						CacheControlMaxAge: -1,
						Parameters: RuleParameters{
							Strategy: "replace",
							Values: map[string][]string{
								"hello": {"world"},
								"foo":   {"bar"},
								"whiz":  {"bang"},
							},
						},
					},
					{
						From:               "example.com/unrecognized-parameter.strategy",
						To:                 "https://foo.com/hello",
						Code:               301,
						compiled:           regexp.MustCompile(""),
						CacheControlMaxAge: 5,
						Parameters: RuleParameters{
							Strategy: "idontexist",
							Values: map[string][]string{
								"hello": {"world"},
								"foo":   {"bar"},
								"whiz":  {"bang"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadConfig(logger, tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadRules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.wantDefaultMiss, got.LocationOnMiss)

			if !cmp.Equal(got.RuleMap, tt.wantRuleMapping, cmpopts.IgnoreFields(Rule{}, "compiled")) {
				t.Errorf("\ngot  = %v\nwant = %v", got.RuleMap, tt.wantRuleMapping)
			}
		})
	}
}

func Test_testFromAsURL(t *testing.T) {
	logger := newTestLogger()

	type args struct {
		url string
	}
	type want struct {
		host  string
		proto string
		path  string
	}
	var testCases = []struct {
		name      string
		args      args
		wantError bool
		want      want
	}{
		{
			name: "leading protocol",
			args: args{
				url: "http://httpbin.org/get",
			},
			want: want{
				host:  "httpbin.org",
				proto: "http",
				path:  "/get",
			},
			wantError: false,
		},
		{
			name: "no leading protocol",
			args: args{
				url: "httpbin.org/get",
			},
			want: want{
				host:  "httpbin.org",
				proto: "https",
				path:  "/get",
			},
			wantError: false,
		},
		{
			name: "no leading protocol with regex",
			args: args{
				url: "httpbin.org/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			want: want{
				host:  "httpbin.org",
				proto: "https",
				path:  "/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			wantError: false,
		},
		{
			name: "leading protocol with regex",
			args: args{
				url: "ssh://httpbin.org/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			want: want{
				host:  "httpbin.org",
				proto: "ssh",
				path:  "/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			wantError: false,
		},
		{
			name: "leading protocol no path",
			args: args{
				url: "http://httpbin.org",
			},
			want: want{
				host:  "httpbin.org",
				proto: "http",
				path:  "/",
			},
			wantError: false,
		},
		{
			name: "no leading protocol no path",
			args: args{
				url: "httpbin.org",
			},
			want: want{
				host:  "httpbin.org",
				proto: "https",
				path:  "/",
			},
			wantError: false,
		},
		{
			name: "relative url",
			args: args{
				url: "/foo/bar",
			},
			want:      want{},
			wantError: true,
		},
		{
			name: "invalid, no hostname",
			args: args{
				url: "http:///",
			},
			want:      want{},
			wantError: true,
		},
		{
			name: "invalid, only protocol",
			args: args{
				url: "http://",
			},
			want:      want{},
			wantError: true,
		},
		{
			name: "invalid, no hostname, additional slashes",
			args: args{
				url: "http://///",
			},
			want:      want{},
			wantError: true,
		},
		{
			name: "invalid, no hostname, junk chars",
			args: args{
				url: "http://$%^$%^^&D^%FC%D^%^&()!H*",
			},
			want:      want{},
			wantError: true,
		},
		{
			name: "junk chars",
			args: args{
				url: "%/^$/%^\\^&D^%FC%D^%^&()!H*",
			},
			want:      want{},
			wantError: true,
		},
		{
			name: "leading protocol with query",
			args: args{
				url: "http://httpbin.org/?foo=bar",
			},
			want: want{
				host:  "httpbin.org",
				proto: "http",
				path:  "/?foo=bar",
			},
			wantError: false,
		},
		{
			name: "leading protocol with regex capture groups",
			args: args{
				url: "http://example.com/test/(?<CAPTURE>\\w+)/(?<GROUP2>\\w+)",
			},
			want: want{
				host:  "example.com",
				proto: "http",
				path:  "/test/(?<CAPTURE>\\w+)/(?<GROUP2>\\w+)",
			},
			wantError: false,
		},
		{
			name: "no leading protocol with regex capture groups",
			args: args{
				url: "example.com/test/(?<CAPTURE>\\w+)/(?<GROUP2>\\w+)",
			},
			want: want{
				host:  "example.com",
				proto: "https",
				path:  "/test/(?<CAPTURE>\\w+)/(?<GROUP2>\\w+)",
			},
			wantError: false,
		},
		{
			name: "no leading protocol with regex no capture groups",
			args: args{
				url: "example.com/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			want: want{
				host:  "example.com",
				proto: "https",
				path:  "/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			wantError: false,
		},
		{
			name: "leading protocol with regex no capture groups",
			args: args{
				url: "http://example.com/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			want: want{
				host:  "example.com",
				proto: "http",
				path:  "/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			wantError: false,
		},
		{
			name: "leading protocol hostname contains dash",
			args: args{
				url: "http://example-test.com/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			want: want{
				host:  "example-test.com",
				proto: "http",
				path:  "/blog/[[:digit:]]{4}/[[:digit:]]{2}/[[:digit:]]{2}/(.+)",
			},
			wantError: false,
		},
		{
			name: "empty string",
			args: args{
				url: "",
			},
			want:      want{},
			wantError: true,
		},
		{
			name: "one char no proto",
			args: args{
				url: "t",
			},
			want: want{
				host:  "t",
				proto: "https",
				path:  "/",
			},
			wantError: false,
		},
		{
			name: "one char with proto",
			args: args{
				url: "http://t",
			},
			want: want{
				host:  "t",
				proto: "http",
				path:  "/",
			},
			wantError: false,
		},
		{
			name: "leading protocol with port",
			args: args{
				url: "http://example.com:8080/test",
			},
			want: want{
				host:  "example.com",
				proto: "http",
				path:  "/test",
			},
			wantError: false,
		},
		{
			name: "no leading protocol with port",
			args: args{
				url: "example.com:8080/test",
			},
			want: want{
				host:  "example.com",
				proto: "https",
				path:  "/test",
			},
			wantError: false,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fromAsURL(logger, tt.args.url)
			if (err != nil) != tt.wantError {
				t.Errorf("fromAsURL()\nerror =   %+v\nwantErr = %+v", err, tt.wantError)
			}
			u := url.URL{
				Scheme: tt.want.proto,
				Host:   tt.want.host,
				Path:   tt.want.path,
			}
			if !cmp.Equal(u, got, cmpopts.IgnoreFields(url.URL{}, "RawPath", "RawQuery", "Fragment", "RawFragment")) {
				t.Errorf("fromAsURL()\ngot =  %+v\nwant = %+v", got, u)
			}
		})
	}
}
