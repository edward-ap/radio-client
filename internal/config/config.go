// Package config defines the MiniRadio configuration format and helpers for
// loading or saving it to disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// AppID is the stable application identifier used for config storage.
	AppID = "miniradio"
	// AppConfigSubdir is the OS-specific directory that holds the config file.
	AppConfigSubdir = "MiniRadio"
	// AppConfigName is the JSON file stored on disk.
	AppConfigName = "config.json"

	// DefaultWidth is the preferred window width when no persisted value exists.
	DefaultWidth = 680
	// DefaultHeight is the preferred (and clamped) window height for the bar UI.
	DefaultHeight = 32
	// DefaultVolume sets the safe initial playback level.
	DefaultVolume = 70
	// DefaultTestURL is a known-good stream used when no preset is configured.
	DefaultTestURL = "https://26413.live.streamtheworld.com/WMGKFMAACIHR.aac"
	// DefaultAPIEndpoint points at the legacy Radio Browser node used previously.
	DefaultAPIEndpoint = "https://de1.api.radio-browser.info"
	// MinWindowWidth keeps right-aligned controls visible even on first launch.
	MinWindowWidth = 860

	// EQNameAuto is a deprecated alias for the built-in "Flat" EQ preset.
	EQNameAuto = "Auto"
	// EQNameOff disables EQ for a preset; it matches the option shown in UI.
	EQNameOff = "Off"
)

// EQPresetData represents a user-created EQ preset stored in the config file.
type EQPresetData struct {
	Name   string    `json:"name"`
	Preamp float32   `json:"preamp"`
	Bands  []float32 `json:"bands"`
}

// Preset describes a single radio station entry available in the UI grid.
type Preset struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	EQ           string `json:"eq,omitempty"`
	MetadataType string `json:"metadataType,omitempty"`
	MetadataURL  string `json:"metadataUrl,omitempty"`
}

// Config aggregates every user-facing preference persisted between sessions.
type Config struct {
	CurrentURL      string         `json:"currentUrl"`
	Volume          int            `json:"volume"`
	Muted           bool           `json:"muted"`
	LastPreset      int            `json:"lastPreset"`
	Presets         [10]Preset     `json:"presets"`
	WindowW         int            `json:"windowW"`
	WindowH         int            `json:"windowH"`
	WindowX         int            `json:"windowX,omitempty"`
	WindowY         int            `json:"windowY,omitempty"`
	WindowPosValid  bool           `json:"windowPosValid,omitempty"`
	ApiEndpoint     string         `json:"apiEndpoint"`
	CustomEQPresets []EQPresetData `json:"customEqPresets,omitempty"`
}

// ConfigDir resolves the writable directory that should contain the config file.
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AppConfigSubdir), nil
}

// ConfigPath is a helper that returns the full path to config.json.
func ConfigPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, AppConfigName), nil
}

// Load reads the config from disk, applying defaults or gently migrating legacy
// values when necessary.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := newDefaultConfig()
			// Try saving an initial config, but still return defaults even if it fails.
			_ = cfg.Save()
			return cfg, nil
		}
		return nil, err
	}

	cfg := &Config{}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("config parse error: %w", err)
	}
	cfg.applyRuntimeDefaults()
	return cfg, nil
}

// Save persists the configuration to disk, creating directories as needed.
func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// AppID returns the stable identifier used by the GUI framework.
func (c *Config) AppID() string { return AppID }

// newDefaultConfig builds an in-memory config populated with safe defaults.
func newDefaultConfig() *Config {
	cfg := &Config{
		CurrentURL:      DefaultTestURL,
		Volume:          DefaultVolume,
		Muted:           false,
		LastPreset:      -1,
		WindowW:         DefaultWidth,
		WindowH:         DefaultHeight,
		ApiEndpoint:     DefaultAPIEndpoint,
		CustomEQPresets: []EQPresetData{},
	}
	cfg.applyRuntimeDefaults()
	for i := range cfg.Presets {
		cfg.Presets[i].EQ = EQNameOff
	}
	return cfg
}

// applyRuntimeDefaults normalizes config values after a load or when defaults
// are constructed, ensuring the UI always receives sane inputs.
func (c *Config) applyRuntimeDefaults() {
	if strings.TrimSpace(c.CurrentURL) == "" {
		c.CurrentURL = DefaultTestURL
	}
	if c.WindowW == 0 {
		c.WindowW = DefaultWidth
	}
	if c.WindowW < MinWindowWidth {
		c.WindowW = MinWindowWidth
	}
	if c.WindowH == 0 {
		c.WindowH = DefaultHeight
	}
	if c.Volume < 0 || c.Volume > 100 {
		c.Volume = DefaultVolume
	}
	if !c.WindowPosValid && (c.WindowX != 0 || c.WindowY != 0) {
		c.WindowPosValid = true
	}
	if strings.TrimSpace(c.ApiEndpoint) == "" {
		c.ApiEndpoint = DefaultAPIEndpoint
	}
	for i := range c.Presets {
		v := strings.TrimSpace(c.Presets[i].EQ)
		if v == "" || strings.EqualFold(v, EQNameAuto) {
			c.Presets[i].EQ = EQNameOff
		}
	}
	if c.CustomEQPresets == nil {
		c.CustomEQPresets = []EQPresetData{}
	}
}
