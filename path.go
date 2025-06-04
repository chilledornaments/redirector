package main

import (
	"net/url"
	"regexp"
)

type StringNotExpandableError struct {
	path string
	exp  string
	to   string
}

func (StringNotExpandableError) Error() string {
	return "StringNotExpandableError"
}

// rewritePath performs a regex substitution, replacing the original request path with the target path
// using the supplied expression
//
// It accepts a request path, a regex, and another regex
func rewritePath(path string, from *regexp.Regexp, to string) (string, error) {
	b := []byte{}
	for _, submatches := range from.FindAllStringSubmatchIndex(path, -1) {
		b = from.ExpandString(b, to, path, submatches)
	}

	if len(b) == 0 {
		return path, StringNotExpandableError{path, from.String(), to}
	}

	p, _ := url.Parse(string(b))

	return p.Path, nil
}
