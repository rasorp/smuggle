package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func main() {
	app := &cli.Command{
		Name:  "smuggle-gateway",
		Usage: "Standalone WireGuard gateway for dev access to a Smuggle overlay network",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "overlay-cidr",
				Usage:   "Full Smuggle overlay CIDR, e.g. 10.100.0.0/16",
				Sources: cli.EnvVars("SMUGGLE_GW_OVERLAY_CIDR"),
			},
			&cli.StringFlag{
				Name:    "tunnel-subnet",
				Usage:   "Subnet to draw customer tunnel IPs from, e.g. 10.200.0.0/24",
				Sources: cli.EnvVars("SMUGGLE_GW_TUNNEL_SUBNET"),
			},
			&cli.StringFlag{
				Name: "endpoint-ip",
				Usage: "IP address of this node reachable by customers " +
					"(public IP or private IP for VPC peering — whatever is routable from the customer's network)",
				Sources: cli.EnvVars("SMUGGLE_GW_ENDPOINT_IP"),
			},
			&cli.StringFlag{
				Name:    "overlay-iface",
				Value:   "smuggle0",
				Usage:   "Name of the existing Smuggle overlay interface",
				Sources: cli.EnvVars("SMUGGLE_GW_OVERLAY_IFACE"),
			},
			&cli.StringFlag{
				Name:    "wg-iface",
				Value:   "wg-gw0",
				Usage:   "WireGuard interface name to create for customer tunnels",
				Sources: cli.EnvVars("SMUGGLE_GW_WG_IFACE"),
			},
			&cli.IntFlag{
				Name:    "wg-port",
				Value:   51820,
				Usage:   "UDP listen port for the customer WireGuard interface",
				Sources: cli.EnvVars("SMUGGLE_GW_WG_PORT"),
			},
			&cli.StringFlag{
				Name:    "wg-key-file",
				Value:   "/tmp/wg-gw0.key",
				Usage:   "Path to persist/load the gateway WireGuard private key",
				Sources: cli.EnvVars("SMUGGLE_GW_WG_KEY_FILE"),
			},
			&cli.StringFlag{
				Name:    "http-addr",
				Value:   "0.0.0.0:9091",
				Usage:   "Address the HTTP API listens on",
				Sources: cli.EnvVars("SMUGGLE_GW_HTTP_ADDR"),
			},
			&cli.BoolFlag{
				Name:  "dev",
				Usage: "Use a human-readable development logger instead of JSON",
			},
		},
		Action: run,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	cfg := &Config{
		OverlayCIDR:  cmd.String("overlay-cidr"),
		TunnelSubnet: cmd.String("tunnel-subnet"),
		EndpointIP:   cmd.String("endpoint-ip"),
		OverlayIface: cmd.String("overlay-iface"),
		WGIface:      cmd.String("wg-iface"),
		WGPort:       cmd.Int("wg-port"),
		WGKeyFile:    cmd.String("wg-key-file"),
		HTTPAddr:     cmd.String("http-addr"),
	}

	if errs := cfg.Validate(); len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "config error: %s\n", err)
		}
		return fmt.Errorf("configuration is invalid")
	}

	logger, err := buildLogger(cmd.Bool("dev"))
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync() //nolint:errcheck

	gw, err := NewGateway(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create gateway: %w", err)
	}
	if err := gw.Start(); err != nil {
		return fmt.Errorf("failed to start gateway: %w", err)
	}

	httpServer := NewHTTPServer(cfg.HTTPAddr, gw, logger)
	if err := httpServer.Start(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	logger.Info("smuggle-gateway running",
		zap.String("http_addr", cfg.HTTPAddr),
		zap.String("wg_iface", cfg.WGIface),
		zap.String("endpoint", fmt.Sprintf("%s:%d", cfg.EndpointIP, cfg.WGPort)),
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	logger.Info("signal received, shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", zap.Error(err))
	}
	if err := gw.Stop(); err != nil {
		logger.Error("gateway stop error", zap.Error(err))
	}

	return nil
}

func buildLogger(dev bool) (*zap.Logger, error) {
	if dev {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}
