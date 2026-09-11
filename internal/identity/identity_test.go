package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func authority(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Voyager identity"}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(50, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	kd, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd})
}

func TestAutomaticRenewalKeepsTrust(t *testing.T) {
	ca, key := authority(t)
	issuer, err := New(ca, key, "voyager.internal")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	first, err := issuer.certificate(now)
	if err != nil {
		t.Fatal(err)
	}
	again, err := issuer.certificate(now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("unnecessary renewal")
	}
	renewed, err := issuer.certificate(now.Add(6 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if first == renewed {
		t.Fatal("certificate did not renew")
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	for _, tc := range []struct {
		cert *tls.Certificate
		now  time.Time
	}{{first, now}, {renewed, now.Add(6 * 24 * time.Hour)}} {
		leaf, err := x509.ParseCertificate(tc.cert.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "voyager.internal", CurrentTime: tc.now}); err != nil {
			t.Fatal(err)
		}
		if leaf.NotAfter.Sub(tc.now) > 8*24*time.Hour {
			t.Fatal("leaf not short lived")
		}
	}
	if _, err := issuer.certificate(now.AddDate(51, 0, 0)); err == nil {
		t.Fatal("expired trust anchor accepted")
	}
}

func TestRejectMismatchedIdentity(t *testing.T) {
	ca, _ := authority(t)
	_, other := authority(t)
	if _, err := New(ca, other, "voyager.internal"); err == nil {
		t.Fatal("mismatched private key accepted")
	}
}
