// Package config loads server settings from the environment and an optional
// .env file. Real environment variables always win over .env values.
package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds every setting the server reads at startup.
type Config struct {
	Port           string
	AllowedOrigins []string
	StaticDir      string

	DatasetCSV  string
	DatasetJSON string

	GroqAPIKey          string
	GroqModel           string
	GroqBaseURL         string
	GroqReasoningEffort string
}

// Load reads settings from the process environment first, then from the
// given .env files in order. Blank values are skipped, so an empty
// GROQ_API_KEY= in one file does not hide a key set in another.
func Load(envFiles ...string) Config {
	fileVals := map[string]string{}
	for _, f := range envFiles {
		m, err := godotenv.Read(f)
		if err != nil {
			continue // a missing file is fine
		}
		for k, v := range m {
			if strings.TrimSpace(v) != "" && fileVals[k] == "" {
				fileVals[k] = v
			}
		}
	}
	get := func(key, fallback string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		if v := strings.TrimSpace(fileVals[key]); v != "" {
			return v
		}
		return fallback
	}

	model := get("GROQ_MODEL", "openai/gpt-oss-120b")
	// Only reasoning models accept reasoning_effort; Groq rejects it for
	// others, so it is dropped rather than sent.
	effort := ""
	if SupportsReasoning(model) {
		effort = get("GROQ_REASONING_EFFORT", "low")
	}

	return Config{
		Port:                get("PORT", "8080"),
		AllowedOrigins:      splitList(get("ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		StaticDir:           get("STATIC_DIR", "../frontend/dist"),
		DatasetCSV:          get("DATASET_CSV", "data/diabetes_dataset.csv"),
		DatasetJSON:         get("DATASET_JSON", "data/dataset.json"),
		GroqAPIKey:          get("GROQ_API_KEY", ""),
		GroqModel:           model,
		GroqBaseURL:         get("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
		GroqReasoningEffort: strings.ToLower(strings.TrimSpace(effort)),
	}
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SupportsReasoning reports whether a Groq model accepts reasoning_effort.
func SupportsReasoning(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "gpt-oss") || strings.Contains(m, "qwen3")
}
