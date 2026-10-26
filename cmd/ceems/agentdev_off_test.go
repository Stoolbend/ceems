//go:build !agentdev

package main

import "testing"

func TestCheckAgentDevListenUntagged(t *testing.T) {
	if err := checkAgentDevListen("0.0.0.0:80"); err != nil {
		t.Fatal(err)
	}
}
