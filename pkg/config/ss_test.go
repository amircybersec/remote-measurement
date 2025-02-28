package config

import (
	"testing"
)

func TestBuildURL(t *testing.T) {
	testCases := []struct {
		name     string
		config   SSConfig
		expected string
	}{
		{
			name: "Full config with prefix",
			config: SSConfig{
				Server:     "admin.c1.havij.co",
				ServerPort: 443,
				Method:     "chacha20-ietf-poly1305",
				Password:   "WhRZ2CeMR5RCgsw1",
				Prefix:     "POST%20x2a8a1eO",
			},
			expected: "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpXaFJaMkNlTVI1UkNnc3cx@admin.c1.havij.co:443?prefix=POST%2520x2a8a1eO",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.config.BuildURL()
			if err != nil {
				t.Fatalf("BuildURL() error = %v", err)
			}
			if got != tc.expected {
				t.Errorf("BuildURL() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestParseSSConfig(t *testing.T) {
	jsonConfig := `{
		"server": "admin.c1.havij.co",
		"server_port": 443,
		"method": "chacha20-ietf-poly1305",
		"password": "WhRZ2CeMR5RCgsw1",
		"prefix": "POST%20x2a8a1eO"
	}`

	expected := "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpXaFJaMkNlTVI1UkNnc3cx@admin.c1.havij.co:443?prefix=POST%2520x2a8a1eO"

	got, err := ParseSSConfig(jsonConfig)
	if err != nil {
		t.Fatalf("ParseSSConfig() error = %v", err)
	}
	if got != expected {
		t.Errorf("ParseSSConfig() = %v, want %v", got, expected)
	}
}
