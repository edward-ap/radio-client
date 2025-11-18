package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadDefaultConfig(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideConfigEnv(tempDir)
	defer restore()

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath error: %v", err)
	}
	_ = os.Remove(path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil config")
	}
	if cfg.CurrentURL != DefaultTestURL {
		t.Errorf("CurrentURL = %q, want %q", cfg.CurrentURL, DefaultTestURL)
	}
	if cfg.Volume != DefaultVolume {
		t.Errorf("Volume = %d, want %d", cfg.Volume, DefaultVolume)
	}
	if cfg.Muted {
		t.Error("Muted should default to false")
	}
	if cfg.LastPreset != -1 {
		t.Errorf("LastPreset = %d, want -1", cfg.LastPreset)
	}
	if cfg.WindowW != MinWindowWidth {
		t.Errorf("WindowW = %d, want %d", cfg.WindowW, MinWindowWidth)
	}
	if cfg.WindowH != DefaultHeight {
		t.Errorf("WindowH = %d, want %d", cfg.WindowH, DefaultHeight)
	}
	if cfg.CustomEQPresets == nil {
		t.Fatal("CustomEQPresets should be initialised")
	}
	for i, preset := range cfg.Presets {
		if preset.EQ != EQNameOff {
			t.Errorf("preset %d EQ = %q, want %q", i, preset.EQ, EQNameOff)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file at %s, got error: %v", path, err)
	}
}

func overrideConfigEnv(tempDir string) func() {
	originals := map[string]string{
		"APPDATA":         os.Getenv("APPDATA"),
		"LOCALAPPDATA":    os.Getenv("LOCALAPPDATA"),
		"USERPROFILE":     os.Getenv("USERPROFILE"),
		"XDG_CONFIG_HOME": os.Getenv("XDG_CONFIG_HOME"),
		"HOME":            os.Getenv("HOME"),
	}

	if runtime.GOOS == "windows" {
		os.Setenv("APPDATA", tempDir)
		os.Setenv("LOCALAPPDATA", tempDir)
		os.Setenv("USERPROFILE", tempDir)
	} else {
		xdg := filepath.Join(tempDir, "xdg")
		_ = os.MkdirAll(xdg, 0o755)
		os.Setenv("XDG_CONFIG_HOME", xdg)
		os.Setenv("HOME", tempDir)
	}

	return func() {
		for k, v := range originals {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}
}
