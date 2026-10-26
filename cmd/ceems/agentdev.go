//go:build agentdev

package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
)

// checkAgentDevListen refuses a listen address whose host is not a loopback IP, because
// this build has the credential-free agent login (ADR 0007). The name localhost is refused.
func checkAgentDevListen(listen string) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("agentdev build: listen %q: %w", listen, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() {
		return fmt.Errorf("agentdev build: listen %q is not a loopback IP", listen)
	}
	// The serve logger does not exist yet when this runs; the default logger writes to stderr too.
	slog.Warn("agentdev build: credential-free agent login is enabled")
	return nil
}
