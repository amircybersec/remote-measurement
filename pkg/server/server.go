package server

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"

	"connectivity-tester/pkg/database"
	"connectivity-tester/pkg/ipinfo"
	"connectivity-tester/pkg/models"
)

func AddServersFromFile(db *database.DB, filename string, serversName string, preresolve bool) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		accessKey := scanner.Text()
		servers, err := parseAccessKey(accessKey, preresolve)
		if err != nil {
			slog.Error("Error parsing access key", "accessKey", accessKey, "error", err)
			continue
		}

		for _, server := range servers {
			slog.Debug("Adding server", "server", server)

			// set server name field
			if serversName != "" {
				server.Name = serversName
			}

			// Get IP info
			ipInfo, err := ipinfo.GetIPInfo(server.IP)
			if err != nil {
				slog.Warn("Error getting IP info", "ip", server.IP, "error", err)
			} else {
				slog.Debug("IP info retrieved", "ip", server.IP, "ipInfo", ipInfo)
				ipinfo.UpdateServerWithIPInfo(&server, ipInfo)
				slog.Debug("Server updated with IP info", "server", server)
			}

			err = db.UpsertServer(context.Background(), &server)
			if err != nil {
				slog.Error("Error upserting server", "accessKey", accessKey, "error", err)
			} else {
				slog.Debug("Server upserted successfully", "accessKey", accessKey)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %v", err)
	}

	return nil
}

func parseAccessKey(accessKey string, preresolve bool) ([]models.Server, error) {
	var servers []models.Server
	parsedURL, err := url.Parse(accessKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse access key: %v", err)
	}

	fragment := parsedURL.Fragment
	parsedURL.Fragment = ""
	fullURLWithoutFragment := parsedURL.String()

	fmt.Printf("Fragment:%s\n", fragment)
	fmt.Printf("FullAccessLink:%s\n", fullURLWithoutFragment)

	// Always resolve URL to get IP addresses
	urls, err := resolveURL(fullURLWithoutFragment)
	if err != nil {
		return nil, err
	}

	err = addTransportInfo(urls)
	if err != nil {
		return nil, err
	}

	server := models.Server{}
	for _, t := range urls.TransportJSON {
		server.IP = t.IP
		server.Port = t.Port
		server.IPType = t.IPVersion
		server.DomainName = t.Host
		server.UserInfo = t.UserInfo
		server.Scheme = t.Scheme
		// If preresolve is false, use the original domain name in the access link
		if !preresolve && server.DomainName != "" {
			// Reconstruct the URL with the original domain
			u := &url.URL{
				Scheme: server.Scheme,
				User:   url.UserPassword(server.UserInfo, ""), // Note: password is empty as it's not stored
				Host:   server.DomainName + ":" + server.Port,
			}
			server.FullAccessLink = u.String()
		} else {
			server.FullAccessLink = t.ResolvedAccessLink
		}
		server.Fragment = fragment
		servers = append(servers, server)
	}
	return servers, nil
}
