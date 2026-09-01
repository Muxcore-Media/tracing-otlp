package internal

import (
	"strings"
)

const redactedValue = "***REDACTED***"

var sensitiveKeyTokens = []string{
	"token",
	"authorization",
	"cookie",
	"password",
	"secret",
}

func redactSpanAttributes(attrs map[string]string) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		if isSensitiveAttributeKey(k) {
			out[k] = redactedValue
		} else {
			out[k] = v
		}
	}
	return out
}

func isSensitiveAttributeKey(key string) bool {
	keyLower := strings.ToLower(key)
	for _, token := range sensitiveKeyTokens {
		if keyMatchesToken(keyLower, token) {
			return true
		}
	}
	return false
}

func keyMatchesToken(key, token string) bool {
	if key == token {
		return true
	}
	for _, seg := range strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	}) {
		if seg == token {
			return true
		}
	}
	return false
}
