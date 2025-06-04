package main

import (
	"net/url"
)

const (
	ParamsStrategyCombine = "combine"
	ParamsStrategyReplace = "replace"
	ParamsStrategyUnset   = ""
)

type UnknownParameterStrategyError struct {
	s string
}

func (u UnknownParameterStrategyError) Error() string {
	return "unknown parameter strategy: " + u.s
}

func buildLocationParams(strategy string, c url.Values, n url.Values) (url.Values, error) {
	switch strategy {
	case ParamsStrategyCombine:
		return combine(c, n)
	case ParamsStrategyReplace:
		return replace(n)
	case ParamsStrategyUnset:
		return url.Values{}, nil
	default:
		return url.Values{}, UnknownParameterStrategyError{s: strategy}
	}
}

// combine combines c and n and returns url.Values. n will overwrite conflicting parameters in c
func combine(c url.Values, n url.Values) (url.Values, error) {
	final := url.Values{}

	for k, v := range c {
		final[k] = v
	}

	for k, v := range n {
		// If a key exists in both the original query parameters and those supplied in `n`, `n` wins
		final[k] = v
	}

	return final, nil
}

// replace discards any existing query parameters and returns only those provided in `newVals`
func replace(n url.Values) (url.Values, error) {
	final := url.Values{}
	for k, v := range n {
		final[k] = v
	}

	return final, nil
}
