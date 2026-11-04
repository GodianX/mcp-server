package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const maxConfigBytes = 1 << 20

type (
	Config struct {
		Server  Server  `yaml:"server"`
		HTTP    HTTP    `yaml:"http"`
		Search  Search  `yaml:"search"`
		Fetch   Fetch   `yaml:"fetch"`
		Logging Logging `yaml:"logging"`
	}
	Server struct {
		Transport             string        `yaml:"transport"`
		Host                  string        `yaml:"host"`
		Port                  int           `yaml:"port"`
		RequestTimeout        time.Duration `yaml:"request_timeout"`
		ShutdownTimeout       time.Duration `yaml:"shutdown_timeout"`
		MaxConcurrentRequests int           `yaml:"max_concurrent_requests"`
		MaxRequestBytes       int64         `yaml:"max_request_bytes"`
	}
	HTTP struct {
		Timeout          time.Duration `yaml:"timeout"`
		MaxResponseBytes int64         `yaml:"max_response_bytes"`
		MaxRedirects     int           `yaml:"max_redirects"`
	}
	Search struct {
		Interval     time.Duration `yaml:"interval"`
		DefaultLimit int           `yaml:"default_limit"`
		MaxLimit     int           `yaml:"max_limit"`
	}
	Fetch struct {
		DefaultMaxChars int `yaml:"default_max_chars"`
		MaxChars        int `yaml:"max_chars"`
	}
	Logging struct {
		Level        slog.Level `yaml:"level"`
		Output       string     `yaml:"output"`
		Directory    string     `yaml:"directory"`
		LogResponses bool       `yaml:"log_responses"`
	}
)

func Default() Config {
	return Config{
		Server: Server{
			Transport:             "http",
			Host:                  "127.0.0.1",
			Port:                  8080,
			RequestTimeout:        45 * time.Second,
			ShutdownTimeout:       5 * time.Second,
			MaxConcurrentRequests: 2,
			MaxRequestBytes:       16384,
		},
		HTTP:    HTTP{Timeout: 20 * time.Second, MaxResponseBytes: 5 << 20, MaxRedirects: 5},
		Search:  Search{Interval: 2 * time.Second, DefaultLimit: 5, MaxLimit: 10},
		Fetch:   Fetch{DefaultMaxChars: 20000, MaxChars: 50000},
		Logging: Logging{Level: slog.LevelInfo, Output: "console", Directory: "logs"},
	}
}

func Load(path string) (Config, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	c := Default()
	d := yaml.NewDecoder(io.LimitReader(f, maxConfigBytes))
	d.KnownFields(true)

	err = d.Decode(&c)
	if err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	var extra any

	err = d.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		return Config{}, errors.New("config must contain one YAML document")
	}

	err = c.Validate()

	return c, err
}

func (c Config) Validate() error {
	for _, validate := range []func() error{c.validateServer, c.validateTimeouts, c.validateLimits, c.Logging.Validate} {
		err := validate()
		if err != nil {
			return err
		}
	}

	return nil
}

func (l Logging) Validate() error {
	if l.Output != "console" && l.Output != "file" {
		return errors.New("logging.output must be console or file")
	}

	if l.Output == "file" && l.Directory == "" {
		return errors.New("logging.directory is required for file output")
	}

	return nil
}

func (c Config) validateServer() error {
	ip := net.ParseIP(c.Server.Host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("server.host must be a loopback IP")
	}

	if c.Server.Transport != "http" && c.Server.Transport != "stdio" {
		return errors.New("server.transport must be http or stdio")
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}

	if c.Server.MaxConcurrentRequests < 1 || c.Server.MaxConcurrentRequests > 16 {
		return errors.New("max_concurrent_requests must be between 1 and 16")
	}

	return nil
}

func (c Config) validateTimeouts() error {
	if c.Server.RequestTimeout <= 0 || c.Server.ShutdownTimeout <= 0 || c.HTTP.Timeout <= 0 || c.Search.Interval <= 0 {
		return errors.New("timeouts and search.interval must be positive")
	}

	return nil
}

func (c Config) validateLimits() error {
	if c.Server.MaxRequestBytes < 1024 || c.Server.MaxRequestBytes > 1<<20 || c.HTTP.MaxResponseBytes < 1024 ||
		c.HTTP.MaxResponseBytes > 32<<20 {
		return errors.New("request or response byte limit out of range")
	}

	if c.HTTP.MaxRedirects < 0 || c.HTTP.MaxRedirects > 10 {
		return errors.New("max_redirects must be between 0 and 10")
	}

	if c.Search.DefaultLimit < 1 || c.Search.DefaultLimit > c.Search.MaxLimit || c.Search.MaxLimit > 20 {
		return errors.New("invalid search limits")
	}

	if c.Fetch.DefaultMaxChars < 1 || c.Fetch.DefaultMaxChars > c.Fetch.MaxChars || c.Fetch.MaxChars > 200000 {
		return errors.New("invalid fetch character limits")
	}

	return nil
}
