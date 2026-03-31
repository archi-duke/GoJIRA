package config

import "os"

var (
	ServerPort   = envOrDefault("GOJIRA_SERVER_PORT", ":9000")
	JiraMockURL  = envOrDefault("GOJIRA_JIRA_URL", "http://localhost:8000")
	JiraAPIBase  = envOrDefault("GOJIRA_JIRA_API_BASE", "/rest/api/2")
	MongoURI     = envOrDefault("GOJIRA_MONGO_URI", "mongodb://localhost:27017")
	MongoDB      = envOrDefault("GOJIRA_MONGO_DB", "gojira")
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
