package main

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

func handleMatchError(err error, w http.ResponseWriter, cache Cache, host string, path string, fallback string) {
	var noRuleForHostError NoRuleForHostError
	var noMatchFoundError NoRuleForPathError

	s := http.StatusNotFound
	var l string

	switch {
	// if we end up handling these cases the same way, this logic should be consolidated
	case errors.As(err, &noRuleForHostError):
		{
			if fallback != "" {
				s = http.StatusTemporaryRedirect
				l = fallback
			}

		}
	case errors.As(err, &noMatchFoundError):
		{
			if fallback != "" {
				s = http.StatusTemporaryRedirect
				l = fallback
			}
		}
	default:
		{
			s = http.StatusInternalServerError
		}
	}

	if l != "" {
		w.Header().Set("Location", l)
	}
	w.WriteHeader(s)

	// TODO should this run in a goroutine?
	_ = cache.Set(CacheSetParameters{
		host:     host,
		path:     path,
		location: l,
		code:     s,
	})
}

func handleRewritePathError(err error, w http.ResponseWriter) {

}

func getTraceID(r *http.Request) (traceID string) {
	// TODO this should look for headers first
	return uuid.New().String()
}

func setCacheControlMaxAge(d int, r int, w http.ResponseWriter) {
	switch r {
	case -1:
		return
	// not set at rule-level, check set globally
	case 0:
		// if not explicitly disabled globally, set to global default
		if d > -1 {
			w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", d))
		}
	default:
		// set to what's configured at the rule-level
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", r))
	}
}

func handleRequest(l *slog.Logger, cache Cache, ac *AppConfig) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			// if port included in Host, strip it out
			if strings.Contains(host, ":") {
				sp := strings.Split(host, ":")
				if len(sp) > 1 {
					host = sp[0]
				}
			}
			path := r.URL.Path
			params := r.URL.Query()

			logger := l.WithGroup("request_handler").With("host", host).With("path", path).With("correlation_id", getTraceID(r))

			cached, err := cache.Get(CacheGetParameters{
				host: host,
				path: path,
			})
			if err != nil {
				logger.Warn("error from cache.Get", "err", err.Error())
			}

			if cached != nil {
				logger.Debug("cache hit", "location", cached.location)
				w.Header().Set("X-Redirector-Cache-Status", "cached")
				w.Header().Set("Location", cached.location)
				setCacheControlMaxAge(ac.CacheControlMaxAge, cached.cacheMaxAge, w)
				w.WriteHeader(cached.code)
				return
			}

			rule, err := findMatch(logger, host, path, ac.RuleMap)
			if err != nil {
				handleMatchError(
					err,
					w,
					cache,
					host,
					path,
					ac.LocationOnMiss)

				return
			}

			p, err := rewritePath(path, rule.compiled, rule.To)

			// There was an error turning the rules 'from' directive into the rule's 'to' directive
			if err != nil {
				// We won't cache this because it's the result of a configuration error
				if ac.LocationOnMiss != "" {
					w.Header().Set("Location", ac.LocationOnMiss)
				}
				w.WriteHeader(ac.StatusOnMiss)
				return
			}

			newParams, err := buildLocationParams(rule.Parameters.Strategy, params, rule.Parameters.Values)
			// this doesn't need its own error handling function because we just eat these errors
			if err != nil {
				switch {
				case errors.As(err, &UnknownParameterStrategyError{}):
					logger.Warn("unknown parameter strategy", "strategy", rule.Parameters.Strategy)
				default:
					logger.Warn("error building location params", "err", err.Error(), "rule", rule)
				}
			}

			location, err := buildLocationHeader(logger, rule.To, p, newParams)
			if err != nil {
				// an error here means we couldn't parse the 'to' directive into a URL, meaning we don't have a Location header to provide,
				// but there _was_ a match
				// as with errors from rewritePath(), this is likely the result of a configuration error, so we won't cache this
				if ac.LocationOnMiss != "" {
					w.Header().Set("Location", ac.LocationOnMiss)
				}
				w.WriteHeader(ac.StatusOnMiss)
				return
			}

			w.Header().Set("Location", location)
			setCacheControlMaxAge(ac.CacheControlMaxAge, rule.CacheControlMaxAge, w)
			w.WriteHeader(rule.Code)

			err = cache.Set(CacheSetParameters{
				host:               host,
				path:               path,
				location:           location,
				code:               rule.Code,
				cacheControlMaxAge: rule.CacheControlMaxAge,
			})
			if err != nil {
				logger.Warn("error from cache.Set", "err", err.Error())
			}

		},
	)
}

func buildLocationHeader(l *slog.Logger, to string, path string, params url.Values) (string, error) {
	parsed, err := url.Parse(to)
	logger := l

	if err != nil {
		logger.Error("error parsing URL while building Location header", "url", to, "err", err)
		return "", err
	}

	location := url.URL{
		Scheme:   parsed.Scheme,
		Host:     parsed.Host,
		Path:     path,
		RawQuery: params.Encode(),
	}

	return location.String(), nil
}
