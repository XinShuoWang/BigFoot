package main

import (
	"testing"
)

func TestCloudConfiguration(t *testing.T) {
	env := map[string]string{"BIGFOOT_MODE": "gate", "PORT": "9000", "BIGFOOT_BEHIND_TLS_PROXY": "true", "BIGFOOT_VOYAGER_URL": "wss://voyager.example/tunnel"}
	get := func(k string) string { return env[k] }
	c, err := parseConfig(nil, get)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "0.0.0.0:9000" || c.Mode != "gate" || !c.BehindTLSProxy {
		t.Fatalf("bad config: mode=%s listen=%s", c.Mode, c.Listen)
	}
	c, err = parseConfig([]string{"-listen", "0.0.0.0:8080"}, get)
	if err != nil || c.Listen != "0.0.0.0:8080" {
		t.Fatal("flags must override env")
	}
}

func TestRejectUnsafeConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{"-mode", "gate", "-voyager-url", "wss://voyager.example/tunnel"},
		{"-mode", "local", "-listen", "0.0.0.0:8080", "-voyager-url", "wss://voyager.example/tunnel"},
		{"-mode", "gate", "-behind-tls-proxy", "-voyager-url", "http://voyager.example"},
		{"-mode", "voyager", "-behind-tls-proxy", "-max-connections", "0"},
		{"-mode", "voyager", "-behind-tls-proxy", "-session-timeout", "0s"},
		{"-mode", "voyager", "-behind-tls-proxy", "-outer-cert", "one.pem"},
		{"-mode", "gate", "-behind-tls-proxy", "-voyager-url", "wss://voyager.example/tunnel", "-ws-path", "/healthz"},
		{"-mode", "gate", "-behind-tls-proxy", "-voyager-url", "wss://voyager.example/tunnel", "-ws-path", "/proxy%2Fsecret"},
		{"-mode", "gate", "-behind-tls-proxy", "-voyager-url", "wss://voyager.example/tunnel", "-ws-path", "/proxy?"},
	} {
		if _, err := parseConfig(args, func(string) string { return "" }); err == nil {
			t.Fatalf("unsafe config accepted: %v", args)
		}
	}
}

func TestCredentialsValidation(t *testing.T) {
	for _, s := range []string{"", "short", "0123456789abcdef0123456789abc defg"} {
		if _, err := parseSecrets(s); err == nil {
			t.Fatal("weak/invalid secret accepted")
		}
	}
	secrets, err := parseSecrets("0123456789abcdef0123456789abcdef\nabcdef0123456789abcdef0123456789ab\n")
	if err != nil || len(secrets) != 2 {
		t.Fatal("rotation secrets rejected")
	}
}

func TestRoutingConfigExclusiveWithLegacyUpstream(t *testing.T) {
	get := func(string) string { return "" }
	if _, err := parseConfig([]string{"-mode", "gate", "-behind-tls-proxy", "-routing-config", "routing.json"}, get); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-mode", "gate", "-behind-tls-proxy", "-routing-config", "routing.json", "-voyager-url", "wss://x/tunnel"},
		{"-mode", "voyager", "-behind-tls-proxy", "-routing-config", "routing.json"},
	} {
		if _, err := parseConfig(args, get); err == nil {
			t.Fatal("conflicting routing accepted")
		}
	}
}
