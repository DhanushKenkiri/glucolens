package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesEnvFiles(t *testing.T) {
	dir := t.TempDir()
	backend := filepath.Join(dir, "backend.env")
	root := filepath.Join(dir, "root.env")
	os.WriteFile(backend, []byte("GROQ_API_KEY=\nGROQ_MODEL=llama-3.3-70b-versatile\nPORT=9000\n"), 0o644)
	os.WriteFile(root, []byte("GROQ_API_KEY=gsk_from_root\nPORT=7000\n"), 0o644)
	for _, k := range []string{"GROQ_API_KEY", "GROQ_MODEL", "PORT", "GROQ_REASONING_EFFORT"} {
		t.Setenv(k, "")
	}

	cfg := Load(backend, root, filepath.Join(dir, "missing.env"))
	if cfg.GroqAPIKey != "gsk_from_root" {
		t.Errorf("a blank key in the first file must not hide the key in the second; got %q", cfg.GroqAPIKey)
	}
	if cfg.Port != "9000" || cfg.GroqModel != "llama-3.3-70b-versatile" {
		t.Errorf("earlier files win: port=%s model=%s", cfg.Port, cfg.GroqModel)
	}
	if cfg.GroqReasoningEffort != "" {
		t.Errorf("non-reasoning models get no reasoning effort, got %q", cfg.GroqReasoningEffort)
	}
	t.Setenv("GROQ_REASONING_EFFORT", "high")
	if cfg := Load(backend, root); cfg.GroqReasoningEffort != "" {
		t.Errorf("reasoning effort must be dropped for llama models, got %q", cfg.GroqReasoningEffort)
	}
	t.Setenv("GROQ_REASONING_EFFORT", "")

	t.Setenv("PORT", "8123")
	t.Setenv("GROQ_MODEL", "openai/gpt-oss-20b")
	cfg = Load(backend, root)
	if cfg.Port != "8123" || cfg.GroqReasoningEffort != "low" {
		t.Errorf("real environment wins: port=%s effort=%q", cfg.Port, cfg.GroqReasoningEffort)
	}
}
