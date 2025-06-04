package main

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"log/slog"
	"sync"
	"time"
)

var (
	cacheHitMetric = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hit",
			Help: "Number of cache hits",
		},
		[]string{"host", "path"},
	)
	cacheMissMetric = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_miss",
			Help: "Number of cache hits",
		},
		[]string{"host", "path"},
	)
	cacheCleanupJobDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "cache_cleanup_job_duration_milliseconds",
			Help: "Duration of cache cleanup job",
		})
)

type Cache interface {
	Get(parameters CacheGetParameters) (*CacheResponse, error)
	Set(parameters CacheSetParameters) error
}

type CacheGetParameters struct {
	host string
	path string
}

type CacheSetParameters struct {
	host               string
	path               string
	location           string
	code               int
	cacheControlMaxAge int
}

type InMemoryCache struct {
	logger *slog.Logger
	ttl    int64
	lock   sync.RWMutex
	// {host: {path: Item}}
	cache map[string]map[string]InMemoryCacheItem
}

type InMemoryCacheItem struct {
	path               string
	location           string
	code               int
	ttl                int64
	createdAt          int64
	cacheControlMaxAge int
}

type CacheResponse struct {
	location    string
	code        int
	cacheMaxAge int
}

func recordCacheMetric(t string, host string, path string) {
	switch t {
	case "hit":
		go func(h string, p string) {
			cacheHitMetric.With(prometheus.Labels{
				"host": h,
				"path": p,
			}).Inc()
		}(host, path)
	case "miss":
		go func(h string, p string) {
			cacheMissMetric.With(prometheus.Labels{
				"host": h,
				"path": p,
			}).Inc()
		}(host, path)
	}
}

func (c *InMemoryCache) Get(parameters CacheGetParameters) (*CacheResponse, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if d, ok := c.cache[parameters.host]; ok {
		if r, ok := d[parameters.path]; ok {
			c.logger.Debug("cache hit for path", "host", parameters.host, "path", parameters.path)
			recordCacheMetric("hit", parameters.host, parameters.path)
			return &CacheResponse{code: r.code, location: r.location, cacheMaxAge: r.cacheControlMaxAge}, nil
		} else {
			c.logger.Debug("path-level cache miss", "host", parameters.host, "path", parameters.path)
			recordCacheMetric("miss", parameters.host, parameters.path)
			return nil, nil
		}
	} else {
		c.logger.Debug("host-level cache miss", "host", parameters.host, "path", parameters.path)
		recordCacheMetric("miss", parameters.host, parameters.path)
		return nil, nil
	}
}

func (c *InMemoryCache) Set(parameters CacheSetParameters) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	item := InMemoryCacheItem{
		path:               parameters.path,
		location:           parameters.location,
		code:               parameters.code,
		ttl:                c.ttl,
		createdAt:          time.Now().Unix(),
		cacheControlMaxAge: parameters.cacheControlMaxAge,
	}

	if _, ok := c.cache[parameters.host]; ok {
		c.cache[parameters.host][parameters.path] = item
	} else {
		c.cache[parameters.host] = make(map[string]InMemoryCacheItem)
		c.cache[parameters.host][parameters.path] = item
	}
	c.logger.Debug("adding item to cache", "host", parameters.host, "path", parameters.path, "code", parameters.code, "ttl", c.ttl, "location", parameters.location)
	return nil
}

func NewInMemoryCache(ctx context.Context, l *slog.Logger, interval int, ttl int64) *InMemoryCache {
	logger := l.WithGroup("cache")
	c := &InMemoryCache{
		logger: logger,
		cache:  make(map[string]map[string]InMemoryCacheItem),
		ttl:    ttl,
	}

	// Start background job to clean up expired records
	go func(ctx context.Context, c *InMemoryCache) {
		for {
			if ctx.Err() != nil {
				logger.Info("stopping cache cleanup")
				return
			}
			start := time.Now().UnixMilli()

			c.logger.Debug("starting cache cleanup")
			// TODO a time-based cache is a lazy way to not have to implement more complex logic while keeping the cache size in check
			for _, domain := range c.cache {
				for path, item := range domain {
					now := time.Now().Unix()
					if now > (item.createdAt + item.ttl) {
						c.logger.Debug("removing expired rule from cache", "path", path, "code", item.code, "location", item.location, "ttl", item.ttl, "now", now)
						c.lock.Lock()
						delete(domain, path)
						c.lock.Unlock()
					}
				}
			}
			end := time.Now().UnixMilli()
			cacheCleanupJobDuration.Observe(float64(end - start))
			c.logger.Debug("finished cache cleanup")
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}(ctx, c)

	return c
}
