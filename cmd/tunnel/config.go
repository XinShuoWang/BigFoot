package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	Mode, Listen, TokenFile, ClientPasswordFile                    string
	InnerIdentityKey                                               string
	InnerCert, InnerKey, InnerCA, InnerName, VoyagerURL            string
	OuterCert, OuterKey, OuterCA, WSPath                           string
	BehindTLSProxy, Check                                          bool
	SessionTimeout, IdleTimeout, HandshakeTimeout, ShutdownTimeout time.Duration
	MaxConnections                                                 int
	getenv                                                         func(string) string
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	c := config{getenv: getenv}
	fs := flag.NewFlagSet("tunnel", flag.ContinueOnError)
	fs.StringVar(&c.Mode, "mode", "", "gate (cloud ingress), voyager (exit), or local (loopback CONNECT)")
	fs.StringVar(&c.Listen, "listen", "", "listen address; defaults to 0.0.0.0:PORT (9000), local 127.0.0.1:8080")
	fs.StringVar(&c.TokenFile, "token-file", "", "Gate/Voyager token file, one secret per line; Gate uses first, Voyager accepts all")
	fs.StringVar(&c.ClientPasswordFile, "client-password-file", "", "Gate Trojan password file, one secret per line")
	fs.StringVar(&c.InnerIdentityKey, "inner-identity-key", "", "Voyager stable identity key PEM; enables automatic leaf certificate renewal")
	fs.StringVar(&c.InnerCert, "inner-cert", "", "Voyager inner TLS certificate PEM")
	fs.StringVar(&c.InnerKey, "inner-key", "", "Voyager inner TLS key PEM")
	fs.StringVar(&c.InnerCA, "inner-ca", "", "Gate inner TLS CA PEM")
	fs.StringVar(&c.InnerName, "inner-name", "voyager.internal", "Voyager inner TLS identity")
	fs.StringVar(&c.VoyagerURL, "voyager-url", "", "Gate upstream wss URL")
	fs.StringVar(&c.OuterCA, "outer-ca", "", "optional CA for outer WSS (system roots by default)")
	fs.StringVar(&c.OuterCert, "outer-cert", "", "direct serving TLS certificate PEM")
	fs.StringVar(&c.OuterKey, "outer-key", "", "direct serving TLS key PEM")
	fs.StringVar(&c.WSPath, "ws-path", "/proxy", "Gate Trojan WebSocket path")
	fs.BoolVar(&c.BehindTLSProxy, "behind-tls-proxy", false, "explicitly trust platform TLS termination; serve HTTP behind trusted gateway")
	fs.BoolVar(&c.Check, "check", false, "validate configuration and credentials without listening")
	fs.DurationVar(&c.SessionTimeout, "session-timeout", 15*time.Minute, "maximum connection lifetime")
	fs.DurationVar(&c.IdleTimeout, "idle-timeout", 2*time.Minute, "maximum application inactivity in both directions")
	fs.DurationVar(&c.HandshakeTimeout, "handshake-timeout", 10*time.Second, "Gate client authentication deadline")
	fs.DurationVar(&c.ShutdownTimeout, "shutdown-timeout", 5*time.Second, "drain deadline before active connections are cancelled")
	fs.IntVar(&c.MaxConnections, "max-connections", 128, "maximum active sessions per instance, including pending authentication")
	var envErr error
	fs.VisitAll(func(f *flag.Flag) {
		if v := getenv("BIGFOOT_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))); v != "" {
			if err := fs.Set(f.Name, v); err != nil {
				envErr = fmt.Errorf("invalid environment setting for %s", f.Name)
			}
		}
	})
	if envErr != nil {
		return c, envErr
	}
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() != 0 {
		return c, errors.New("unexpected positional arguments")
	}
	if c.Mode != "gate" && c.Mode != "voyager" && c.Mode != "local" {
		return c, errors.New("mode must be gate, voyager or local")
	}
	if c.Listen == "" {
		c.Listen = "0.0.0.0:9000"
		if port := getenv("PORT"); port != "" {
			c.Listen = net.JoinHostPort("0.0.0.0", port)
		}
		if c.Mode == "local" {
			c.Listen = "127.0.0.1:8080"
		}
	}
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return c, errors.New("invalid listen address")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return c, errors.New("invalid listen port")
	}
	if c.Mode == "local" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return c, errors.New("local mode requires a numeric loopback address")
		}
	}
	if (c.OuterCert == "") != (c.OuterKey == "") {
		return c, errors.New("outer-cert and outer-key must be configured together")
	}
	if c.Mode != "local" && c.OuterCert == "" && !c.BehindTLSProxy {
		return c, errors.New("cloud mode requires outer TLS or explicit behind-tls-proxy")
	}
	if c.OuterCert != "" && c.BehindTLSProxy {
		return c, errors.New("choose direct TLS or platform TLS termination, not both")
	}
	if c.MaxConnections < 1 || c.SessionTimeout <= 0 || c.IdleTimeout <= 0 || c.HandshakeTimeout <= 0 || c.ShutdownTimeout <= 0 {
		return c, errors.New("connection limits and timeouts must be positive")
	}
	if c.Mode != "voyager" {
		u, e := url.Parse(c.VoyagerURL)
		if e != nil || u.Scheme != "wss" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return c, errors.New("voyager-url must be a wss URL without userinfo or fragment")
		}
		if c.InnerName == "" {
			return c, errors.New("inner-name is required")
		}
	}
	u, e := url.ParseRequestURI(c.WSPath)
	if e != nil || !strings.HasPrefix(c.WSPath, "/") || strings.HasPrefix(c.WSPath, "//") || u.RawQuery != "" || u.RawPath != "" || u.ForceQuery || u.Path != c.WSPath || u.Fragment != "" || c.WSPath == "/healthz" || c.WSPath == "/readyz" {
		return c, errors.New("ws-path must be an absolute path other than health endpoints")
	}
	return c, nil
}

// material supports mounted secret files or platform-provided environment secrets.
// Never place secret values in flags, error messages or logs.
func (c config) material(path, env string) ([]byte, error) {
	value := c.getenv(env)
	if path != "" && value != "" {
		return nil, fmt.Errorf("configure a file or %s, not both", env)
	}
	if path == "" {
		if value == "" {
			return nil, fmt.Errorf("missing secret/certificate: %s or its file flag", env)
		}
		if len(value) > 65536 {
			return nil, errors.New("credential too large")
		}
		return []byte(value), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read credential file: %w", err)
	}
	defer f.Close()
	p, err := io.ReadAll(io.LimitReader(f, 65537))
	if len(p) > 65536 {
		return nil, errors.New("credential too large")
	}
	return p, err
}

func parseSecrets(value string) ([]string, error) {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) > 16 {
		return nil, errors.New("at most 16 rotation secrets are supported")
	}
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if len(line) < 32 || len(line) > 256 {
			return nil, errors.New("each secret must contain 32 to 256 printable non-space ASCII bytes")
		}
		for _, r := range line {
			if r < 33 || r > 126 {
				return nil, errors.New("secret contains whitespace or non-ASCII bytes")
			}
		}
		lines[i] = line
	}
	return lines, nil
}

func (c config) secrets(path, env string) ([]string, error) {
	p, err := c.material(path, env)
	if err != nil {
		return nil, err
	}
	return parseSecrets(string(p))
}
func (c config) pool(path, env string) (*x509.CertPool, error) {
	p, err := c.material(path, env)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(p) {
		return nil, errors.New("no CA certificates in trust store")
	}
	return roots, nil
}
func (c config) innerCertificate() (tls.Certificate, error) {
	certPEM, err := c.material(c.InnerCert, "BIGFOOT_INNER_CERT_PEM")
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := c.material(c.InnerKey, "BIGFOOT_INNER_KEY_PEM")
	if err != nil {
		return tls.Certificate{}, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return cert, err
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return cert, err
	}
	if time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return cert, errors.New("inner certificate is not currently valid")
	}
	if err := leaf.VerifyHostname(c.InnerName); err != nil {
		return cert, fmt.Errorf("inner certificate identity: %w", err)
	}
	return cert, nil
}
