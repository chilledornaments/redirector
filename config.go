package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

const (
	defaultStatusCode                 = http.StatusMovedPermanently
	defaultParameterStrategy          = ParamsStrategyCombine
	defaultListenAddress              = "0.0.0.1:8484"
	defaultMetricsServerListenAddress = "0.0.0.1:8485"
	defaultCacheTTL                   = 86400
	defaultCacheCleanupInterval       = 3600
	defaultLocationOnMiss             = ""
	defaultStatusOnMiss               = http.StatusNotFound
	defaultCacheControlMaxAge         = 86400 * 7 // cache for one week
)

type AppConfig struct {
	lock                       sync.RWMutex
	ListenAddress              string      `yaml:"listen_address"`
	MetricsServerListenAddress string      `yaml:"metrics_server_listen_address"`
	LocationOnMiss             string      `yaml:"location_on_miss"`
	StatusOnMiss               int         `yaml:"status_on_miss"`
	DefaultParameterStrategy   string      `yaml:"default_parameter_strategy"`
	CacheControlMaxAge         int         `yaml:"cache_control_max_age"`
	Cache                      CacheConfig `yaml:"cache"`
	RuleMap                    RuleMapping
	Rules                      `yaml:"rules"`
}

type CacheConfig struct {
	TTL             int64 `yaml:"ttl"`
	CleanupInterval int   `yaml:"cleanup_interval"`
}

// RuleMapping maps a hostname to a list of Rule objects
type RuleMapping map[string]Rules

type Rules []Rule

type Rule struct {
	From               string         `yaml:"from"`
	To                 string         `yaml:"to"`
	Code               int            `yaml:"code"`
	Parameters         RuleParameters `yaml:"parameters"`
	CacheControlMaxAge int            `yaml:"cache_control_max_age"`
	compiled           *regexp.Regexp
}

type RuleParameters struct {
	Strategy string              `yaml:"strategy"`
	Values   map[string][]string `yaml:"values"`
}

type InvalidConfigError struct{}

func (e InvalidConfigError) Error() string {
	return "Invalid configuration"
}

func loadConfig(l *slog.Logger, path string) (*AppConfig, error) {
	// Set defaults
	c := &AppConfig{
		ListenAddress:              defaultListenAddress,
		CacheControlMaxAge:         defaultCacheControlMaxAge,
		MetricsServerListenAddress: defaultMetricsServerListenAddress,
		DefaultParameterStrategy:   defaultParameterStrategy,
		LocationOnMiss:             defaultLocationOnMiss,
		StatusOnMiss:               defaultStatusOnMiss,

		Cache: CacheConfig{
			TTL:             defaultCacheTTL,
			CleanupInterval: defaultCacheCleanupInterval,
		},
	}

	c.lock.Lock()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buffer, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	// Unmarshalling here yields a config without bucketed rules, but does contain the rest of the settings
	err = yaml.Unmarshal(buffer, c)
	if err != nil {
		return nil, err
	}

	rules := buildRules(l, &c.Rules, defaultStatusCode, c.DefaultParameterStrategy, c.CacheControlMaxAge)
	bucketed := bucketRules(l, rules)

	c.RuleMap = bucketed
	c.lock.Unlock()

	return c, nil
}

// buildRules returns a pointer to a Rules object that contains only valid rules with configured behavior and compiled expressions
//
// Invalid rules will be logged and dropped from returned object
func buildRules(l *slog.Logger, r *Rules, c int, s string, a int) *Rules {
	n := Rules{}

	logger := l.WithGroup("config")
	logger.Debug("validating config", "rules", r)

	for _, rule := range *r {
		u, err := fromAsURL(logger, rule.From)
		if err != nil {
			// don't load rule if we can't convert to a URL
			logger.Warn("not loading rule, from directive cannot be converted to URL", "rule", fmt.Sprintf("+%v", rule), "err", err)
			continue
		}

		if !strings.Contains(rule.To, "://") {
			logger.Warn("not loading rule, to directive missing protocol", "rule", fmt.Sprintf("+%v", rule))
			continue
		}

		var exp *regexp.Regexp
		var compileErr error
		// if _only_ the hostname was provided, we'll assume this is a blanket redirect for any request
		if u.Path == "" {
			exp, compileErr = regexp.Compile("^.*")
		} else {
			p := u.Path
			// anchor all paths if not already anchored in order to guarantee behavior that one would expect
			// out of the box, which is to say if I declare `to: foo.com/bar`, I don't want it to match 'foo.com/x/y/z/bar',
			// I only want it to match `/bar...`
			if string(p[0]) != "^" {
				p = "^" + p
			}
			exp, compileErr = regexp.Compile(p)
		}

		if compileErr != nil {
			logger.WithGroup("config").Warn("invalid regexp, skipping", "regexp", u.Path, "host", u.Host, "err", compileErr)
			continue
		}

		rule.compiled = exp

		if rule.Code == 0 {
			rule.Code = c
		}

		if rule.Parameters.Strategy == "" {
			rule.Parameters.Strategy = s
		}

		// if unset at the rule-level, we'll set it to the default value
		if rule.CacheControlMaxAge == 0 {
			rule.CacheControlMaxAge = a
		}
		n = append(n, rule)
	}

	return &n
}

// TODO if we need to sub-bucket by first path part, we can do like so:
/*
splitFrom := strings.Split(rule.From, "/")
if len(splitFrom[1:]) > 1 {
	// spitting example.com/foo/bar will yield {example.com, foo, bar}
	// while example.com/ will yield {example.com, ""}, so we can always append a "/" and cover both cases
	//
	// we'll only bucket the first `/<foo>`
 	pathKey := splitFrom[0] + "/"

	bucketedRules[hostname][pathKey] = Rules{}
}

*/

func validHostname(l *slog.Logger, hostname string) bool {
	logger := l
	validSpecialChars := []string{
		"_", "-", ".",
	}
	validSpecialCharsBytes := []byte{}
	for _, char := range validSpecialChars {
		validSpecialCharsBytes = append(validSpecialCharsBytes, []byte(char)...)
	}

	for _, char := range hostname {
		letter := unicode.IsLetter(char)
		digit := unicode.IsDigit(char)
		special := bytes.Contains(validSpecialCharsBytes, []byte(string(char)))
		// if the char is neither a letter, not digit, nor allowed special character, it's invalid
		if !letter && !digit && !special {
			logger.Warn("invalid hostname", "hostname", hostname, "char", string(char))
			return false
		}
	}
	return true
}

type InvalidHostnameError struct {
	h string
}

func (e InvalidHostnameError) Error() string {
	return fmt.Sprintf("invalid hostname: %s", e.h)
}

type InvalidURLError struct {
	u string
}

func (e InvalidURLError) Error() string {
	return fmt.Sprintf("invalid URL: %s", e.u)
}

// fromAsURL converts a rule's from directive into a url.URL object
//
// This is basically just an error handler wrapped around `url.Parse()`
func fromAsURL(l *slog.Logger, f string) (url.URL, error) {
	u := url.URL{}
	logger := l

	// This is a naive check, but is acceptable for what we're trying to do
	// namely: don't force users from specifying the protocol; set a sane default
	protocolSet := strings.Contains(f, "://")
	if !protocolSet {
		f = "https://" + f
	}

	// have to work around certain expressions being treated as query params
	escaped := url.PathEscape(f)
	// Unescape forward slash otherwise we'll receive a parsing error if there is a colon in the paths
	escaped = strings.Replace(escaped, "%2F", "/", -1)
	parsed, err := url.Parse(escaped)
	if err != nil {
		pathSegmentError := strings.Contains(err.Error(), "first path segment in URL cannot contain colon")
		if !pathSegmentError {
			logger.Warn("unable to parse URL", "url", f, "err", err)
			return u, err
		}

		logger.Warn("swallowing first path segment colon error", "url", f, "err", err)
	}

	if parsed == nil {
		logger.Error("unable to parse URL", "url", f, "err", err)
		return url.URL{}, InvalidURLError{f}
	}

	// parsed.Opaque will contain a value if a regex was included in the path
	if parsed.Opaque != "" {
		escaped, err = url.PathUnescape(parsed.Opaque)
		if err != nil {
			logger.Warn("unable to parse URL", "url", f, "err", err)
			return url.URL{}, err
		}
		transparent, err := url.PathUnescape(escaped)
		if err != nil {
			logger.Warn("unable to parse URL", "url", f, "err", err)
			return url.URL{}, err
		}

		if len(transparent) == 0 {
			logger.Warn("unable to parse 0 character URL", "url", f)
			return url.URL{}, InvalidURLError{f}
		}

		sp := strings.Split(transparent, "/")

		if len(sp) <= 2 {
			if len(sp) == 0 {
				return url.URL{}, InvalidURLError{f}
			}
			// if sp only contains 1 item, we'll assume it's a hostname
			// we'll assume that we were passed just a hostname
			// handle only hostname being provided,
			// we'll convert it to
			parsed.Host = sp[0]
			logger.Debug("converting schema-less and path-less URL to full URL", "url", f, "transparent", transparent)
		}

		// update parsed struct so that we can run the rest of our logic
		// the rest of the items in `sp` will be the request path
		// when joining, we need to prepend a slice, otherwise we get "foo/bar" instead of "/foo/bar"
		parsed.Path = "/" + strings.Join(sp[1:], "/")
		parsed.Opaque = ""
	}

	u.Scheme = "https"
	if parsed.Scheme != "" {
		u.Scheme = parsed.Scheme
	}

	u.Path = parsed.Path
	u.RawPath = parsed.RawPath

	u.Host = parsed.Host
	sp := strings.Split(parsed.Path, "/")
	// handle protocol missing in from directive
	if parsed.Host == "" {
		// parsed.Host won't contain scheme, so we can safely assume the first item in the list is the hostname
		u.Host = sp[0]
	}
	// handle from directive being "example.com"
	if len(sp) > 1 {
		j := "/" + strings.Join(sp[1:], "/")
		// We use Path everywhere else, but update RawPath to be consistent. Not sure if this is a smart move
		u.Path = j
		u.RawPath = j
	} else {
		u.Path = "/"
	}

	// handle relative path and missing hostname
	// if we have "https:///...", we have no hostname
	invalid := fmt.Sprintf("%s:///", u.Scheme)
	combined := u.Scheme + "://" + u.Host + u.Path
	substr := combined[0:len(invalid)]
	if substr == invalid {
		return url.URL{}, InvalidHostnameError{h: ""}
	}

	if u.Host != "" {
		if strings.Contains(u.Host, ":") {
			u.Host = strings.Split(u.Host, ":")[0]
		}
		if !validHostname(logger, u.Host) {
			return url.URL{}, InvalidHostnameError{u.Host}
		}
	}

	return u, nil
}

// bucketedRules organizes rules into per-hostname buckets in order to reduce time spent searching for matches
//
// Within a hostname bucket, compiled expressions are mapped to a Rule object
// TODO should we just consolidate this into `buildRules()`? Would save another iteration and speed up config load time
func bucketRules(l *slog.Logger, r *Rules) RuleMapping {
	bucketedRules := RuleMapping{}
	logger := l

	for _, rule := range *r {
		u, err := fromAsURL(logger, rule.From)
		if err != nil {
			// don't load rule if we can't convert to a URL
			logger.Warn("not loading rule", "rule", fmt.Sprintf("+%v", rule), "err", err)
			continue
		}

		if _, ok := bucketedRules[u.Host]; !ok {
			bucketedRules[u.Host] = Rules{rule}
		} else {
			curr := bucketedRules[u.Host]
			curr = append(curr, rule)
			bucketedRules[u.Host] = curr
		}
		logger.Debug("loaded rule", "rule", fmt.Sprintf("+%v", rule), "host", u.Host)
	}

	return bucketedRules

}

// reloader watches the config file and reloads rules if the config file changes
func reloader(ctx context.Context, l *slog.Logger, f string, ac *AppConfig) {
	logger := l.WithGroup("reloader").With("config_path", f)
	logger.Info("starting config reloader")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("failed to create file watcher", "err", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(f)
	if err != nil {
		logger.Error("failed to watch file", "err", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down config reload worker")
			return
		case _, ok := <-watcher.Events:
			if ok {
				cfg, err := loadConfig(logger, f)
				if err != nil {
					logger.Error("error reloading config, reusing existing config", "err", err)
				} else {
					// TODO bust cache
					// TODO this runs twice - is that just IDE double-saving?
					ac.RuleMap = cfg.RuleMap
					logger.Info("reloaded config")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				logger.Error("error watching file but continuing to try", "err", err)
			}
		}
	}
}
