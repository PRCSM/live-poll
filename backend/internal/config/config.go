package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Env            string
	Port           string
	MongoURI       string
	MongoDB        string
	RedisURL       string
	FrontendOrigin string
	JWTSecret      []byte
	VoterSecret    []byte
	TrustedProxies []string
	// ClientIPHeader names a header the hosting edge sets to the real client IP
	// (and overwrites if a client sends it). Empty means use the socket address.
	ClientIPHeader string
}

func (c Config) Production() bool { return c.Env == "production" }

// Load reads configuration from the environment and fails on anything missing,
// so a misconfigured deploy crashes at boot instead of on the first request.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(get func(string) string) (Config, error) {
	c := Config{
		Env:            withDefault(get("APP_ENV"), "development"),
		Port:           withDefault(get("PORT"), "8080"),
		MongoURI:       get("MONGO_URI"),
		MongoDB:        withDefault(get("MONGO_DB"), "livepoll"),
		RedisURL:       get("REDIS_URL"),
		FrontendOrigin: strings.TrimRight(get("FRONTEND_ORIGIN"), "/"),
		JWTSecret:      []byte(get("JWT_SECRET")),
		VoterSecret:    []byte(get("VOTER_SECRET")),
		ClientIPHeader: get("CLIENT_IP_HEADER"),
	}
	if p := get("TRUSTED_PROXIES"); p != "" {
		for _, s := range strings.Split(p, ",") {
			if s = strings.TrimSpace(s); s != "" {
				c.TrustedProxies = append(c.TrustedProxies, s)
			}
		}
	}

	var errs []error
	if c.Env != "development" && c.Env != "production" {
		errs = append(errs, fmt.Errorf("APP_ENV must be development or production, got %q", c.Env))
	}
	for name, v := range map[string]string{"MONGO_URI": c.MongoURI, "REDIS_URL": c.RedisURL, "FRONTEND_ORIGIN": c.FrontendOrigin} {
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
	}
	if len(c.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes"))
	}
	if len(c.VoterSecret) < 32 {
		errs = append(errs, errors.New("VOTER_SECRET must be at least 32 bytes"))
	}
	return c, errors.Join(errs...)
}

func withDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
