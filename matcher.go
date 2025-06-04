package main

import (
	"fmt"
	"log/slog"
)

type NoRuleForHostError struct {
	h string
}

func (n NoRuleForHostError) Error() string {
	return fmt.Sprintf("no rules declared for '%s'", n.h)
}

type NoRuleForPathError struct {
	h string
	p string
}

func (n NoRuleForPathError) Error() string {
	return fmt.Sprintf("no match for host '%s' with path '%s'", n.h, n.p)
}

// findMatch returns the redirect destination, the regular expression that matches the request path, the winning score, and an error
//
// If there is no match, an error is returned
// findMatch assumes `rules` is not empty
func findMatch(l *slog.Logger, hostname string, path string, rules RuleMapping) (Rule, error) {
	winner := Rule{}
	logger := l.WithGroup("matcher")

	if _, ok := rules[hostname]; !ok {
		logger.Warn("no rules for hostname")
		return winner, NoRuleForHostError{h: hostname}
	}

	for _, rule := range rules[hostname] {
		if rule.compiled != nil {
			prefix, _ := rule.compiled.LiteralPrefix()
			if prefix == path {
				winner = rule
				logger.Info("found exact match", "exp", rule.compiled.String(), "path", path)
				break
			}

			rule.compiled.Longest()

			if rule.compiled.MatchString(path) {
				winner = rule
				logger.Info("found regex match", "exp", rule.compiled.String(), "path", path)
				break
			}
		}
	}

	if winner.compiled == nil {
		return winner, NoRuleForPathError{}
	}

	logger.Debug(fmt.Sprintf("winning rule '%s'", winner.compiled.String()), "location", winner.To)

	return winner, nil
}
