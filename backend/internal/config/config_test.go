package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func valid() map[string]string {
	return map[string]string{
		"MONGO_URI":       "mongodb://localhost:27017",
		"REDIS_URL":       "redis://localhost:6379",
		"FRONTEND_ORIGIN": "http://localhost:5173/",
		"JWT_SECRET":      strings.Repeat("a", 32),
		"VOTER_SECRET":    strings.Repeat("b", 32),
		"TRUSTED_PROXIES": " 10.0.0.0/8, ,100.64.0.0/10",
	}
}

func TestLoadValid(t *testing.T) {
	c, err := load(env(valid()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Port != "8080" || c.MongoDB != "livepoll" || c.Env != "development" {
		t.Errorf("defaults not applied: %+v", c)
	}
	if c.FrontendOrigin != "http://localhost:5173" {
		t.Errorf("trailing slash not trimmed: %q", c.FrontendOrigin)
	}
	if len(c.TrustedProxies) != 2 {
		t.Errorf("trusted proxies = %v", c.TrustedProxies)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]struct {
		key, value, want string
	}{
		"missing mongo":  {"MONGO_URI", "", "MONGO_URI is required"},
		"short jwt":      {"JWT_SECRET", "short", "JWT_SECRET must be at least 32 bytes"},
		"short voter":    {"VOTER_SECRET", "short", "VOTER_SECRET"},
		"bad env":        {"APP_ENV", "staging", "APP_ENV"},
		"missing origin": {"FRONTEND_ORIGIN", "", "FRONTEND_ORIGIN is required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := valid()
			m[tc.key] = tc.value
			_, err := load(env(m))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}
