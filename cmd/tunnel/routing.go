package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"

	"local/tunnel-prototype/internal/ingress"
	"local/tunnel-prototype/internal/routing"
	"local/tunnel-prototype/internal/tunnel"
)

var nodeID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
var envID = regexp.MustCompile(`^BIGFOOT_NODE_[A-Z0-9_]+_(TOKEN|CA_PEM)$`)

type upstream struct {
	url, token string
	tls        *tls.Config
}

func (c config) routedHandler() (http.Handler, error) {
	file, err := os.Open(c.RoutingConfig)
	if err != nil {
		return nil, errors.New("cannot open routing configuration")
	}
	defer file.Close()
	cfg, err := routing.Decode(file)
	if err != nil {
		return nil, err
	}
	table, err := routing.Compile(cfg)
	if err != nil {
		return nil, err
	}
	passwords := []string{}
	if c.Mode == "gate" {
		passwords, err = c.secrets(c.ClientPasswordFile, "BIGFOOT_CLIENT_PASSWORD")
		if err != nil {
			return nil, err
		}
	}
	nodes := map[string]upstream{}
	identities := map[string]bool{}
	for name, node := range cfg.Nodes {
		if !nodeID.MatchString(name) || !envID.MatchString(node.TokenEnv) || !envID.MatchString(node.CAEnv) {
			return nil, errors.New("invalid node credential reference")
		}
		u, e := url.Parse(node.URL)
		if e != nil || u.Scheme != "wss" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return nil, errors.New("invalid Voyager WSS endpoint")
		}
		token, err := c.secrets("", node.TokenEnv)
		if err != nil {
			return nil, err
		}
		for _, p := range passwords {
			for _, secret := range token {
				if p == secret {
					return nil, errors.New("client password must differ from Voyager token")
				}
			}
		}
		material, err := c.material("", node.CAEnv)
		if err != nil {
			return nil, err
		}
		block, rest := pem.Decode(material)
		if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
			return nil, errors.New("each Voyager must have one trust certificate")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA {
			return nil, errors.New("invalid Voyager trust certificate")
		}
		fingerprint := string(cert.RawSubjectPublicKeyInfo)
		if identities[fingerprint] {
			return nil, errors.New("Voyager identities must be independent")
		}
		identities[fingerprint] = true
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(material) {
			return nil, errors.New("invalid Voyager trust certificate")
		}
		nodes[name] = upstream{url: node.URL, token: token[0], tls: &tls.Config{RootCAs: roots, ServerName: "voyager.internal", MinVersion: tls.VersionTLS13}}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if c.OuterCA != "" || c.getenv("BIGFOOT_OUTER_CA_PEM") != "" {
		roots, err := c.pool(c.OuterCA, "BIGFOOT_OUTER_CA_PEM")
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	client := &http.Client{Transport: transport}
	dial := func(ctx context.Context, target string) (net.Conn, error) {
		name, err := table.Select(target)
		if err != nil {
			return nil, err
		}
		node := nodes[name]
		return tunnel.Dial(ctx, node.url, target, node.token, node.tls, client)
	}
	if c.Mode == "local" {
		return tunnel.ConnectHandler(dial), nil
	}
	return (&ingress.Server{Passwords: passwords, Path: c.WSPath, HandshakeTimeout: c.HandshakeTimeout, Dial: dial}).Handler(), nil
}
