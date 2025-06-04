//go:build unit_test

package main

import (
	"context"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestSimpleParameterCombine(t *testing.T) {
	logger := newTestLogger()
	ctx := t.Context()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	cfg, _ := loadConfig(logger, "./fixtures/rules.yml")
	cache := NewInMemoryCache(ctx, logger, cfg.Cache.CleanupInterval, cfg.Cache.TTL)

	expected, _ := url.Parse("https://demo.localhost.com/?new=hello&existing=world")

	req := httptest.NewRequest("GET", "http://localhost/params/test?existing=hello", nil)
	w := httptest.NewRecorder()

	handleRequest(logger, cache, cfg).ServeHTTP(w, req)

	resp, _ := url.Parse(w.Header().Get("Location"))

	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, expected.Scheme, resp.Scheme)
	assert.Equal(t, expected.Host, resp.Host)
	assert.Equal(t, expected.Path, resp.Path)

	for k, v := range expected.Query() {
		assert.Equal(t, v, resp.Query()[k])
	}
}

func TestSimpleParameterReplace(t *testing.T) {
	logger := newTestLogger()
	ctx := t.Context()
	ctx, cancel := context.WithCancel(ctx)
	cfg, _ := loadConfig(logger, "./fixtures/rules.yml")
	t.Cleanup(cancel)
	expected, _ := url.Parse("https://demo.localhost.com/?new=hello")

	req := httptest.NewRequest("GET", "http://localhost/params/test2?new=first&existing=hello", nil)
	w := httptest.NewRecorder()

	cache := NewInMemoryCache(ctx, logger, cfg.Cache.CleanupInterval, cfg.Cache.TTL)

	handleRequest(logger, cache, cfg).ServeHTTP(w, req)

	resp, _ := url.Parse(w.Header().Get("Location"))

	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, expected.Scheme, resp.Scheme)
	assert.Equal(t, expected.Host, resp.Host)
	assert.Equal(t, expected.Path, resp.Path)
	assert.Equal(t, len(expected.Query()), len(resp.Query()))

	for k, v := range expected.Query() {
		assert.Equal(t, v, resp.Query()[k])
	}
}

// TestCacheFunctionality verifies that the important parts of the cache work as expected

// This tests: retrieving and setting cache keys; the functionality of the cache cleanup job; retrieving after the cache cleanup job
func TestCacheFunctionality(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	ctx := t.Context()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	cfg, _ := loadConfig(logger, "./fixtures/rules.yml")
	expected, _ := url.Parse("https://demo.localhost.com/?new=hello")

	req := httptest.NewRequest("GET", "http://localhost/params/test2?new=first&existing=hello", nil)
	w := httptest.NewRecorder()

	cache := NewInMemoryCache(ctx, logger, cfg.Cache.CleanupInterval, cfg.Cache.TTL)

	handleRequest(logger, cache, cfg).ServeHTTP(w, req)

	params := CacheGetParameters{req.Host, req.URL.Path}
	cached, _ := cache.Get(params)
	assert.NotNil(t, cached)

	// make sure value from cache is what we expect
	handleRequest(logger, cache, cfg).ServeHTTP(w, req)
	resp, _ := url.Parse(w.Header().Get("Location"))
	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, expected.Scheme, resp.Scheme)
	assert.Equal(t, expected.Host, resp.Host)
	assert.Equal(t, expected.Path, resp.Path)
	assert.Equal(t, len(expected.Query()), len(resp.Query()))

	// wait for TTL to expire so cleanup job can run
	time.Sleep(5 * time.Second)
	cached, err := cache.Get(params)
	assert.Nil(t, err)
	assert.Nil(t, cached)

	// Ensure there are no issues retrieving after cleanup job removes key
	handleRequest(logger, cache, cfg).ServeHTTP(w, req)
	resp, _ = url.Parse(w.Header().Get("Location"))
	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, expected.Scheme, resp.Scheme)
	assert.Equal(t, expected.Host, resp.Host)
	assert.Equal(t, expected.Path, resp.Path)
	assert.Equal(t, len(expected.Query()), len(resp.Query()))
}

func TestPortInToDirective(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	ctx := t.Context()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	cfg, _ := loadConfig(logger, "./fixtures/rules.yml")
	expected := url.URL{
		Host:   "demo.localhost.com:8080",
		Path:   "/foo",
		Scheme: "https",
	}

	req := httptest.NewRequest("GET", "http://localhost/port", nil)
	w := httptest.NewRecorder()

	cache := NewInMemoryCache(ctx, logger, cfg.Cache.CleanupInterval, cfg.Cache.TTL)

	handleRequest(logger, cache, cfg).ServeHTTP(w, req)
	resp, _ := url.Parse(w.Header().Get("Location"))
	assert.Equal(t, defaultStatusCode, w.Code)
	assert.Equal(t, expected.Scheme, resp.Scheme)
	assert.Equal(t, expected.Host, resp.Host)
	assert.Equal(t, expected.Path, resp.Path)
}

func Test_parameterHandling(t *testing.T) {
	t.Parallel()
	logger := newTestLogger()
	ctx := t.Context()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	cfg, _ := loadConfig(logger, "./fixtures/rules.yml")
	type args struct {
		u string
	}
	var testCases = []struct {
		name string
		want string
		args args
	}{
		{
			name: "empty parameters directive",
			args: args{
				u: "http://localhost/empty-param",
			},
			want: "http://foo",
		},
		{
			name: "empty parameters directive with param in to",
			args: args{
				u: "http://localhost/param-in-directive-empty",
			},
			want: "http://foo",
		},
		{
			name: "parameters directive unset with param in to",
			args: args{
				u: "http://localhost/param-in-directive",
			},
			want: "http://foo",
		},
		{
			name: "parameters directive unset with param in to",
			args: args{
				u: "http://localhost/param-in-directive-with-parameters-set",
			},
			want: "http://foo?foo=bar",
		},
	}

	for _, testCase := range testCases {
		req := httptest.NewRequest("GET", testCase.args.u, nil)
		w := httptest.NewRecorder()
		cache := NewInMemoryCache(ctx, logger, 1, 10)

		t.Run(testCase.name, func(t *testing.T) {
			handleRequest(logger, cache, cfg).ServeHTTP(w, req)

			assert.Equal(t, testCase.want, w.Header().Get("Location"))

		})
	}

}
