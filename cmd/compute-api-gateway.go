package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrok-operator/internal/computeapi"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
)

func init() {
	rootCmd.AddCommand(computeAPIGatewayCmd())
}

func computeAPIGatewayCmd() *cobra.Command {
	var healthListen, upstream, tokenFile, caFile string
	var endpointFile, accessKeyHashFile string
	var serverAddr, rootCAs string
	cmd := &cobra.Command{
		Use: "compute-api-gateway",
		RunE: func(cmd *cobra.Command, _ []string) error {
			gateway, err := computeapi.NewGateway(upstream, tokenFile, accessKeyHashFile, caFile)
			if err != nil {
				return err
			}

			var ready atomic.Bool
			healthMux := http.NewServeMux()
			healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
				if !ready.Load() {
					http.Error(w, "ngrok endpoint is not ready", http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
			})
			healthServer := &http.Server{Addr: healthListen, Handler: healthMux, ReadHeaderTimeout: 10 * time.Second}
			healthErr := make(chan error, 1)
			go func() {
				if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					healthErr <- fmt.Errorf("serve compute API gateway health checks: %w", err)
				}
			}()

			listener, err := computeapi.Listen(cmd.Context(), computeapi.EndpointConfig{
				URLFile: endpointFile, Authtoken: os.Getenv("NGROK_AUTHTOKEN"),
				ConnectURL: serverAddr, RootCAs: rootCAs,
				Logger: slog.New(logr.ToSlogHandler(ctrl.Log.WithName("compute-api").WithName("agent"))),
			})
			if err != nil {
				_ = healthServer.Shutdown(context.Background())
				return err
			}
			defer listener.Agent().Disconnect()
			ready.Store(true)

			server := &http.Server{Handler: gateway, ReadHeaderTimeout: 10 * time.Second}
			proxyErr := make(chan error, 1)
			go func() {
				if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
					proxyErr <- fmt.Errorf("serve Compute Kubernetes API over ngrok: %w", err)
				}
			}()

			select {
			case <-cmd.Context().Done():
				ready.Store(false)
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = server.Shutdown(shutdownCtx)
				_ = healthServer.Shutdown(shutdownCtx)
				return nil
			case err := <-healthErr:
				return err
			case err := <-proxyErr:
				ready.Store(false)
				return err
			}
		},
	}
	cmd.Flags().StringVar(&healthListen, "health-listen", ":8081", "Address on which health probes listen")
	cmd.Flags().StringVar(&upstream, "kubernetes-api", "https://kubernetes.default.svc", "Kubernetes API URL")
	cmd.Flags().StringVar(&tokenFile, "token-file", "/var/run/secrets/kubernetes.io/serviceaccount/token", "Projected ServiceAccount token")
	cmd.Flags().StringVar(&caFile, "ca-file", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "Kubernetes CA bundle")
	cmd.Flags().StringVar(&endpointFile, "endpoint-file", "/etc/ngrok/compute-api/endpoint", "Internal ngrok endpoint URL file")
	cmd.Flags().StringVar(&accessKeyHashFile, "access-key-hash-file", "/etc/ngrok/compute-api/access-key-sha256", "SHA-256 verifier for the Compute access key")
	cmd.Flags().StringVar(&serverAddr, "server-addr", "", "The address of the ngrok server to use for the endpoint")
	cmd.Flags().StringVar(&rootCAs, "root-cas", "trusted", "trusted (default) or host: use the trusted ngrok agent CA or the host CA")
	return cmd
}
