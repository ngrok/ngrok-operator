package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ngrok/ngrok-operator/internal/computeapi"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(computeAPIGatewayCmd())
}

func computeAPIGatewayCmd() *cobra.Command {
	var listen, upstream, tokenFile, caFile string
	cmd := &cobra.Command{
		Use: "compute-api-gateway",
		RunE: func(cmd *cobra.Command, _ []string) error {
			gateway, err := computeapi.NewGateway(upstream, tokenFile, caFile)
			if err != nil {
				return err
			}
			server := &http.Server{Addr: listen, Handler: gateway, ReadHeaderTimeout: 10 * time.Second}
			go func() {
				<-cmd.Context().Done()
				_ = server.Shutdown(context.Background())
			}()
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("serve compute API gateway: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&listen, "listen", ":8080", "Address on which the gateway listens")
	cmd.Flags().StringVar(&upstream, "kubernetes-api", "https://kubernetes.default.svc", "Kubernetes API URL")
	cmd.Flags().StringVar(&tokenFile, "token-file", "/var/run/secrets/kubernetes.io/serviceaccount/token", "Projected ServiceAccount token")
	cmd.Flags().StringVar(&caFile, "ca-file", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "Kubernetes CA bundle")
	return cmd
}
