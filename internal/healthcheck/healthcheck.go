// healthcheck is a package that provides a simple health check interface for services via a singleton instance
package healthcheck

import (
	"context"
	"net/http"
)

// checkRequest is a function that checks the health of a service.
type checkRequest func(context.Context, *http.Request) error

// HealthChecker is the interface that wraps the basic health check methods.
type HealthChecker interface {
	// Alive checks if the service is alive (maps to livenessProbe)
	Alive(context.Context, *http.Request) error

	// Ready checks if the service is ready to serve traffic (maps to readinessProbe)
	Ready(context.Context, *http.Request) error
}

// healthcheck stores the internal state of configured HealthCheckers
type healthcheck struct {
	// readyProbes is a list of functions that check if the service is ready to serve traffic
	readyProbes []checkRequest

	// aliveProbes is a list of functions that check if the service is alive
	aliveProbes []checkRequest
}

// instance is a singleton instance of the healthcheck struct
var instance = &healthcheck{
	readyProbes: []checkRequest{},
	aliveProbes: []checkRequest{},
}

// RegisterHealthChecker registers a HealthChecker to the healthcheck instance
func RegisterHealthChecker(hc HealthChecker) {
	instance.readyProbes = append(instance.readyProbes, hc.Ready)
	instance.aliveProbes = append(instance.aliveProbes, hc.Alive)
}

// Alive checks if the registered HealthCheckers are all alive
// If any of the HealthCheckers are not alive, an their error is returned and the remaining HealthCheckers are not checked
func Alive(ctx context.Context, r *http.Request) error {
	return instance.checkAlive(ctx, r)
}

// Ready checks if the registered HealthCheckers are all ready to service traffic
// If any of the HealthCheckers are not ready, an their error is returned and the remaining HealthCheckers are not checked
func Ready(ctx context.Context, r *http.Request) error {
	return instance.checkReady(ctx, r)
}

// checkAlive checks if the registered HealthCheckers are all alive
// If any of the HealthCheckers are not alive, an their error is returned and the remaining HealthCheckers are not checked
func (hc *healthcheck) checkAlive(ctx context.Context, r *http.Request) error {
	for _, probe := range hc.aliveProbes {
		if err := probe(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// checkReady checks if the registered HealthCheckers are all ready to service traffic
// If any of the HealthCheckers are not ready, an their error is returned and the remaining HealthCheckers are not checked
func (hc *healthcheck) checkReady(ctx context.Context, r *http.Request) error {
	for _, probe := range hc.readyProbes {
		if err := probe(ctx, r); err != nil {
			return err
		}
	}
	return nil
}
