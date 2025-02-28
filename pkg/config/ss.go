package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SSConfig represents the shadowsocks configuration structure
type SSConfig struct {
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Method     string `json:"method"`
	Password   string `json:"password"`
	Prefix     string `json:"prefix"`
}

// BuildURL converts the SSConfig into a shadowsocks URL
func (c *SSConfig) BuildURL() (string, error) {
	// Create userinfo by base64 encoding "method:password"
	userInfo := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.Method, c.Password)))

	// Construct URL using net/url
	u := &url.URL{
		Scheme: "ss",
		User:   url.User(userInfo),
		Host:   fmt.Sprintf("%s:%d", c.Server, c.ServerPort),
	}

	// Add prefix as query parameter if it exists
	if c.Prefix != "" {
		q := url.Values{}
		q.Add("prefix", c.Prefix)
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}

// ParseSSConfig parses a JSON string into an SSConfig and returns the URL
func ParseSSConfig(jsonConfig string) (string, error) {
	var config SSConfig
	if err := json.Unmarshal([]byte(jsonConfig), &config); err != nil {
		return "", fmt.Errorf("failed to parse JSON config: %w", err)
	}

	return config.BuildURL()
}

// FetchSSConfig fetches and parses SS configuration from a URL
func FetchSSConfig(configURL string) (string, error) {
	// Parse the input URL
	u, err := url.Parse(configURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Validate URL scheme
	if u.Scheme != "ssconfig" {
		return "", fmt.Errorf("invalid URL scheme: must be ssconfig://")
	}

	// Override scheme to https
	u.Scheme = "https"

	// Fetch the content
	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("failed to fetch config: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	content := strings.TrimSpace(string(body))

	// Check if content is a shadowsocks URL
	if strings.HasPrefix(content, "ss://") {
		return content, nil
	}

	// Try parsing as JSON
	return ParseSSConfig(content)
}
