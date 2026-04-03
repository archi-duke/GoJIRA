package config

import (
	"bufio"
	"os"
	"strings"
)

var (
	ServerPort  = envOrDefault("PLATFORM_SERVER_PORT", ":9000")
	MongoURI    = envOrDefault("PLATFORM_MONGO_URI", "mongodb://localhost:27017")
	MongoDB     = envOrDefault("PLATFORM_MONGO_DB", "gojira")
	GitHubToken = envOrDefault("PLATFORM_GITHUB_TOKEN", "")
	GitHubOrg   = envOrDefault("PLATFORM_GITHUB_ORG", "")
	GitHubAPI   = envOrDefault("PLATFORM_GITHUB_API", "https://api.github.com")
)

func init() {
	loadEnvFile(".env")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
