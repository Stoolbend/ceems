//go:build !agentdev

package main

// checkAgentDevListen accepts any address: only an agentdev build restricts it (ADR 0007).
func checkAgentDevListen(string) error { return nil }
