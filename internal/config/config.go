package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/sudabon/subcli/internal/provider"
)

const (
	DefaultMaxConcurrency = 2
)

// Config holds the validated local configuration for subcli.
type Config struct {
	Providers ProvidersConfig
}

// ProvidersConfig restricts configuration to exactly one entry per supported
// provider so multi-account definitions cannot be configured.
type ProvidersConfig struct {
	Claude ProviderConfig
	Codex  ProviderConfig
	Agy    ProviderConfig
}

// ProviderConfig holds settings for a single provider.
type ProviderConfig struct {
	Auth           provider.AuthMode
	APIKeyEnv      string
	MaxConcurrency int
}

type rawConfig struct {
	Providers rawProvidersConfig `toml:"providers"`
}

type rawProvidersConfig struct {
	Claude *rawProviderConfig `toml:"claude"`
	Codex  *rawProviderConfig `toml:"codex"`
	Agy    *rawProviderConfig `toml:"agy"`
}

type rawProviderConfig struct {
	Auth           string `toml:"auth"`
	APIKeyEnv      string `toml:"api_key_env"`
	MaxConcurrency *int   `toml:"max_concurrency"`
}

// Default returns the safe default configuration used when config.toml does not exist.
func Default() Config {
	return Config{
		Providers: ProvidersConfig{
			Claude: ProviderConfig{
				Auth:           provider.AuthSubscription,
				APIKeyEnv:      "ANTHROPIC_API_KEY",
				MaxConcurrency: DefaultMaxConcurrency,
			},
			Codex: ProviderConfig{
				Auth:           provider.AuthSubscription,
				APIKeyEnv:      "CODEX_API_KEY",
				MaxConcurrency: DefaultMaxConcurrency,
			},
			Agy: ProviderConfig{
				Auth:           provider.AuthSubscription,
				APIKeyEnv:      "GEMINI_API_KEY",
				MaxConcurrency: DefaultMaxConcurrency,
			},
		},
	}
}

// DefaultPath returns $XDG_CONFIG_HOME/subcli/config.toml or the OS standard
// user configuration directory path when XDG_CONFIG_HOME is unset.
func DefaultPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "subcli", "config.toml"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("設定ディレクトリの取得に失敗しました: %w", err)
	}
	return filepath.Join(dir, "subcli", "config.toml"), nil
}

// Load reads and validates the configuration from DefaultPath.
// If the file does not exist, Default() is returned without error.
func Load() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	return LoadFromPath(path)
}

// LoadFromPath reads and validates configuration from path.
// If path does not exist, Default() is returned without error.
func LoadFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("設定ファイル %s の読み込みに失敗しました: %w", path, err)
	}
	cfg, err := LoadFromReader(bytes.NewReader(data))
	if err != nil {
		return Config{}, fmt.Errorf("設定ファイル %s が不正です: %w", path, err)
	}
	return cfg, nil
}

// LoadFromReader decodes and validates TOML configuration from r.
// Unknown fields are strictly rejected to prevent multi-account definitions
// or raw API key storage.
func LoadFromReader(r io.Reader) (Config, error) {
	var raw rawConfig
	dec := toml.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		var strictErr *toml.StrictMissingError
		if errors.As(err, &strictErr) {
			return Config{}, fmt.Errorf("未知の設定項目は許可されていません（1 プロバイダ 1 アカウントのみ対応）: %w", err)
		}
		return Config{}, fmt.Errorf("TOML 解析エラー: %w", err)
	}

	cfg := Default()
	var err error
	if raw.Providers.Claude != nil {
		cfg.Providers.Claude, err = mergeProvider("claude", cfg.Providers.Claude, raw.Providers.Claude)
		if err != nil {
			return Config{}, err
		}
	}
	if raw.Providers.Codex != nil {
		cfg.Providers.Codex, err = mergeProvider("codex", cfg.Providers.Codex, raw.Providers.Codex)
		if err != nil {
			return Config{}, err
		}
	}
	if raw.Providers.Agy != nil {
		cfg.Providers.Agy, err = mergeProvider("agy", cfg.Providers.Agy, raw.Providers.Agy)
		if err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func mergeProvider(name string, base ProviderConfig, raw *rawProviderConfig) (ProviderConfig, error) {
	out := base
	if raw.Auth != "" {
		mode := provider.AuthMode(raw.Auth)
		if !mode.IsValid() {
			return ProviderConfig{}, fmt.Errorf("providers.%s.auth=%q は不正です（%q または %q を指定してください）", name, raw.Auth, provider.AuthSubscription, provider.AuthAPIKey)
		}
		out.Auth = mode
	}
	if raw.APIKeyEnv != "" {
		if err := validateEnvVarName(name, raw.APIKeyEnv); err != nil {
			return ProviderConfig{}, err
		}
		out.APIKeyEnv = raw.APIKeyEnv
	}
	if raw.MaxConcurrency != nil {
		if *raw.MaxConcurrency <= 0 {
			return ProviderConfig{}, fmt.Errorf("providers.%s.max_concurrency=%d は 1 以上の整数である必要があります", name, *raw.MaxConcurrency)
		}
		out.MaxConcurrency = *raw.MaxConcurrency
	}
	return out, nil
}

func validateEnvVarName(providerName, envName string) error {
	if strings.HasPrefix(envName, "sk-") || strings.HasPrefix(envName, "AIza") {
		return fmt.Errorf("providers.%s.api_key_env には API キーの実体ではなく環境変数名を指定してください", providerName)
	}
	if strings.ContainsAny(envName, "= \t\r\n") {
		return fmt.Errorf("providers.%s.api_key_env=%q は有効な環境変数名ではありません", providerName, envName)
	}
	return nil
}

// Provider returns the ProviderConfig for the specified provider ID.
func (c Config) Provider(id string) (ProviderConfig, error) {
	switch id {
	case "claude":
		return c.Providers.Claude, nil
	case "codex":
		return c.Providers.Codex, nil
	case "agy":
		return c.Providers.Agy, nil
	default:
		return ProviderConfig{}, fmt.Errorf("未対応のプロバイダです: %q（claude, codex, agy を指定してください）", id)
	}
}
