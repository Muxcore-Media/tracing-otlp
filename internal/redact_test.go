package internal

import (
	"testing"
)

func TestIsSensitiveAttributeKey(t *testing.T) {
	cases := map[string]bool{
		"authorization": true,
		"http.authorization": true,
		"user_password":      true,
		"cookie":             true,
		"session_token":      true,
		"component":          false,
	}
	for key, want := range cases {
		if got := isSensitiveAttributeKey(key); got != want {
			t.Errorf("isSensitiveAttributeKey(%q) = %v, want %v", key, got, want)
		}
	}
}
