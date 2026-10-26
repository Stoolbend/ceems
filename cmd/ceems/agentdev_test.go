//go:build agentdev

package main

import "testing"

func TestCheckAgentDevListen(t *testing.T) {
	for _, bad := range []string{"0.0.0.0:8080", "192.168.1.2:8080", "localhost:8080", "[::]:8080"} {
		if checkAgentDevListen(bad) == nil {
			t.Errorf("%s: accepted, want an error", bad)
		}
	}
	for _, good := range []string{"127.0.0.1:0", "[::1]:0"} {
		if err := checkAgentDevListen(good); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
}
