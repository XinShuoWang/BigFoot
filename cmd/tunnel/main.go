package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"local/tunnel-prototype/internal/identity"
	"local/tunnel-prototype/internal/ingress"
	"local/tunnel-prototype/internal/service"
	"local/tunnel-prototype/internal/tunnel"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(logger); err != nil && !errors.Is(err, flag.ErrHelp) {
		logger.Error("service failed", "error", err)
		os.Exit(1)
	}
}

func makeHandler(c config) (http.Handler, error) {
	tokens, err := c.secrets(c.TokenFile, "BIGFOOT_TOKEN")
	if err != nil {
		return nil, err
	}
	if c.Mode == "voyager" {
		if c.InnerIdentityKey != "" || c.getenv("BIGFOOT_INNER_IDENTITY_KEY_PEM") != "" {
			if c.InnerCert != "" || c.InnerKey != "" || c.getenv("BIGFOOT_INNER_CERT_PEM") != "" || c.getenv("BIGFOOT_INNER_KEY_PEM") != "" {
				return nil, errors.New("choose automatic identity or static inner certificate, not both")
			}
			ca, err := c.material(c.InnerCA, "BIGFOOT_INNER_CA_PEM")
			if err != nil {
				return nil, err
			}
			key, err := c.material(c.InnerIdentityKey, "BIGFOOT_INNER_IDENTITY_KEY_PEM")
			if err != nil {
				return nil, err
			}
			issuer, err := identity.New(ca, key, c.InnerName)
			if err != nil {
				return nil, err
			}
			return (&tunnel.Server{TLSConfig: issuer.TLSConfig(), Tokens: tokens, SessionTimeout: c.SessionTimeout}).Handler(), nil
		}
		cert, err := c.innerCertificate()
		if err != nil {
			return nil, err
		}
		return (&tunnel.Server{TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}, Tokens: tokens, SessionTimeout: c.SessionTimeout}).Handler(), nil
	}
	roots, err := c.pool(c.InnerCA, "BIGFOOT_INNER_CA_PEM")
	if err != nil {
		return nil, err
	}
	inner := &tls.Config{RootCAs: roots, ServerName: c.InnerName, MinVersion: tls.VersionTLS13}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Direct Gate->Voyager routing; do not inherit ambient HTTP_PROXY credentials/routing.
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if c.OuterCA != "" || c.getenv("BIGFOOT_OUTER_CA_PEM") != "" {
		pool, err := c.pool(c.OuterCA, "BIGFOOT_OUTER_CA_PEM")
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	client := &http.Client{Transport: transport}
	dial := func(ctx context.Context, target string) (net.Conn, error) {
		return tunnel.Dial(ctx, c.VoyagerURL, target, tokens[0], inner, client)
	}
	if c.Mode == "local" {
		return tunnel.ConnectHandler(dial), nil
	}
	passwords, err := c.secrets(c.ClientPasswordFile, "BIGFOOT_CLIENT_PASSWORD")
	if err != nil {
		return nil, err
	}
	for _, password := range passwords {
		for _, token := range tokens {
			if password == token {
				return nil, errors.New("client password must differ from Gate/Voyager token")
			}
		}
	}
	return (&ingress.Server{Passwords: passwords, Path: c.WSPath, HandshakeTimeout: c.HandshakeTimeout, Dial: dial}).Handler(), nil
}

func run(logger *slog.Logger) error {
	c, err := parseConfig(os.Args[1:], os.Getenv)
	if err != nil {
		return err
	}
	handler, err := makeHandler(c)
	if err != nil {
		return err
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.OuterCert != "" {
		cert, err := tls.LoadX509KeyPair(c.OuterCert, c.OuterKey)
		if err != nil {
			return err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if c.Check {
		fmt.Println("configuration valid")
		return nil
	}
	manager := service.New(service.Options{MaxConnections: c.MaxConnections, SessionTimeout: c.SessionTimeout, IdleTimeout: c.IdleTimeout, Logger: logger})
	server := &http.Server{Addr: c.Listen, Handler: manager.Wrap(handler), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024, TLSConfig: tlsConfig}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		if c.OuterCert != "" {
			done <- server.ServeTLS(listener, "", "")
		} else {
			done <- server.Serve(listener)
		}
	}()
	logger.Info("service started", "mode", c.Mode, "listen", c.Listen, "max_connections", c.MaxConnections)
	select {
	case err = <-done:
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()
		manager.Shutdown(cancelCtx)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	logger.Info("draining", "active", manager.Active())
	grace, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
	manager.Shutdown(grace)
	cancel()
	// Manager explicitly cancels upgraded sessions; net/http alone doesn't.
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		server.Close()
	}
	logger.Info("service stopped")
	return nil
}
