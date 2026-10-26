//go:build !agentdev

package httpapi

import "net/http"

// registerAgentLogin adds nothing: the agent login exists only in an agentdev build (ADR 0007).
func (a *api) registerAgentLogin(*http.ServeMux) {}
