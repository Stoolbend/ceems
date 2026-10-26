// Package config loads and validates the JSON configuration file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
)

// Config is the validated server configuration.
type Config struct {
	Listen   string `json:"listen"`
	DataDir  string `json:"dataDir"`
	PagesDir string `json:"pagesDir"`
	LogLevel string `json:"logLevel"`
	// SecureCookies marks the session cookie Secure; set it when served over TLS.
	SecureCookies bool `json:"secureCookies"`
	// GoogleKeyFile is the path of a Google service-account key; empty turns the Sheets source off.
	GoogleKeyFile string `json:"googleKeyFile"`
	// TrustedProxies lists addresses or CIDR prefixes of reverse proxies whose
	// X-Forwarded-For header is believed. List only the proxy itself: a listed
	// address can impersonate any client IP.
	TrustedProxies []string `json:"trustedProxies"`
	// TrustedProxyPrefixes is TrustedProxies parsed by Load.
	TrustedProxyPrefixes []netip.Prefix `json:"-"`
}

// Load reads and validates the config file at path. Every error names the
// file and, where one applies, the key.
func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("%s: %w", path, errors.Unwrap(err))
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("%s: %s", path, decodeMessage(err))
	}

	for _, k := range []struct{ key, val string }{{"listen", c.Listen}, {"dataDir", c.DataDir}, {"pagesDir", c.PagesDir}} {
		if k.val == "" {
			return c, fmt.Errorf("%s: %s: required", path, k.key)
		}
	}
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return c, fmt.Errorf("%s: listen: %v", path, err)
	}
	switch c.LogLevel {
	case "":
		c.LogLevel = "info"
	case "debug", "info", "warn", "error":
	default:
		return c, fmt.Errorf("%s: logLevel: must be debug, info, warn or error, got %q", path, c.LogLevel)
	}
	if c.GoogleKeyFile != "" {
		// Stat only: the key's contents are read by the Sheets library alone.
		fi, err := os.Stat(c.GoogleKeyFile)
		if err != nil {
			return c, fmt.Errorf("%s: googleKeyFile: %w", path, errors.Unwrap(err))
		}
		if !fi.Mode().IsRegular() {
			return c, fmt.Errorf("%s: googleKeyFile: not a regular file", path)
		}
	}
	for i, e := range c.TrustedProxies {
		pfx, err := netip.ParsePrefix(e)
		if err != nil {
			addr, aerr := netip.ParseAddr(e)
			if aerr != nil {
				return c, fmt.Errorf("%s: trustedProxies[%d]: %q is not an IP address or CIDR prefix", path, i, e)
			}
			pfx = netip.PrefixFrom(addr, addr.BitLen())
		}
		c.TrustedProxyPrefixes = append(c.TrustedProxyPrefixes, pfx)
	}
	return c, nil
}

func decodeMessage(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return fmt.Sprintf("%s: must be a %s", typeErr.Field, typeErr.Type.Kind())
	}
	if key, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return "unknown key " + key
	}
	return err.Error()
}
