// Package config loads server settings from environment variables.
//
// For local development, values can also come from a .env file (KEY=VALUE per
// line). Real environment variables always win over the file, so the same
// binary runs unchanged on a hosting platform such as Render.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds every setting the server reads at startup.
type Config struct {
	Port      string // HTTP port to listen on
	MongoURI  string // MongoDB connection string
	RedisURI  string // host:port, or a redis:// / rediss:// URL
	RedisPass string // only used when RedisURI is host:port
	JWTSecret string // HMAC key for signing access tokens
	LogFile   string // empty = log to stdout
	LogLevel  string // logrus level name (debug, info, warn, ...)
}

// Cfg is the loaded configuration, set once by Load at startup.
var Cfg Config

// Load reads the optional env file at path, then builds Cfg from the environment.
func Load(path string) error {
	envDir, err := loadEnvFile(path)
	if err != nil {
		return err
	}

	Cfg = Config{
		Port:      getEnv("PORT", "8080"),
		MongoURI:  getEnv("DB_URI", "mongodb://echo:echo@localhost:27017"),
		RedisURI:  getEnv("REDIS_URI", "localhost:6379"),
		RedisPass: os.Getenv("REDIS_PASS"),
		JWTSecret: strings.Trim(strings.TrimSpace(os.Getenv("JWT_SECRET")), `"'`),
		LogFile:   os.Getenv("LOG_FILE"),
		LogLevel:  getEnv("LOG_LEVEL", "debug"),
	}

	// A relative log path is relative to the project (where .env lives),
	// not to whichever folder the server was started from.
	if Cfg.LogFile != "" && !filepath.IsAbs(Cfg.LogFile) && envDir != "" {
		Cfg.LogFile = filepath.Join(envDir, Cfg.LogFile)
	}

	if Cfg.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// findEnvFile returns path if it exists; otherwise, for a relative path, it
// looks in each parent directory so the server finds the project's .env even
// when started from a subfolder (e.g. an IDE running cmd/server directly).
func findEnvFile(path string) string {
	if _, err := os.Stat(path); err == nil || filepath.IsAbs(path) {
		return path
	}
	dir, err := os.Getwd()
	if err != nil {
		return path
	}
	for {
		candidate := filepath.Join(dir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return path
		}
		dir = parent
	}
}

// loadEnvFile copies KEY=VALUE pairs from path into the process environment
// without overriding variables that are already set, and returns the directory
// the file was found in. A missing file is not an error (it returns "").
func loadEnvFile(path string) (string, error) {
	path = findEnvFile(path)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, strings.TrimSpace(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}
