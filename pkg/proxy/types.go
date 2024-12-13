package proxy

import "connectivity-tester/pkg/models"

// System represents the type of proxy system
type System string

const (
	SystemSOAX      System = "soax"
	SystemProxyRack System = "proxyrack"
	SystemNone      System = "none"
)

type Config struct {
	System        System
	Username      string
	APIKey        string
	PackageID     string
	PackageKey    string
	SessionLength int
	Endpoint      string
	MaxWorkers    int
}

// Provider defines the interface for different proxy providers
type Provider interface {
	GetISPList(countryISO string, clientType models.ClientType) ([]string, error)
	GetClientForISP(isp string, clientType models.ClientType, country string, maxRetries int) (*models.Client, error)
	BuildTransportURL(client *models.Client) string
	GetProviderName() string
	IsValidClient(client *models.Client) (bool, error)
	GetSessionLength() int
	GetMaxWorkers() int
}
