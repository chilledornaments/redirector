//go:build unit_test

package main

import (
	"log/slog"
	"reflect"
	"testing"
)

func Test_findMatch(t *testing.T) {
	rules := newTestRules("./fixtures/rules.yml")
	logger := newTestLogger()

	type args struct {
		logger        *slog.Logger
		hostname      string
		path          string
		rules         RuleMapping
		exactWins     bool
		paramStrategy string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "simple",
			args: args{
				logger:    logger,
				path:      "/test/foo/hello",
				hostname:  "example.com",
				rules:     rules,
				exactWins: true,
			},
			want:    "https://foo.com/bar/$GROUP2/$CAPTURE",
			wantErr: false,
		},
		{
			name: "no match",
			args: args{
				logger:    logger,
				path:      "/no-match",
				hostname:  "example.com",
				rules:     rules,
				exactWins: true,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "first-longest-match-only-exact",
			args: args{
				logger:    logger,
				path:      "/test/longest/path",
				hostname:  "example.com",
				rules:     rules,
				exactWins: true,
			},
			want:    "https://foo.com/bar/$GROUP2/$CAPTURE",
			wantErr: false,
		},
		{
			name: "blog fixture",
			args: args{
				logger:    logger,
				path:      "/blog/2020/01/01/foo/post",
				hostname:  "localhost",
				rules:     rules,
				exactWins: true,
			},
			want:    "https://blog.localhost.com/posts/$1",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := findMatch(tt.args.logger, tt.args.hostname, tt.args.path, tt.args.rules)
			if (err != nil) != tt.wantErr {
				t.Errorf("findMatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(rule.To, tt.want) {
				t.Errorf("findMatch() to = %v, want %v", rule.To, tt.want)
			}
		})
	}
}
