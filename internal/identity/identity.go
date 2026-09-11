// Package identity automatically renews Voyager's TLS certificates under a stable,
// Voyager-specific trust anchor. Gate only receives the public anchor, never its key.
package identity

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"sync"
	"time"
)

type Issuer struct {
	mu      sync.Mutex
	ca      *x509.Certificate
	key     crypto.Signer
	name    string
	current *tls.Certificate
}

func New(caPEM, keyPEM []byte, name string) (*Issuer, error) {
	pair, err := tls.X509KeyPair(caPEM, keyPEM)
	if err != nil {
		return nil, errors.New("invalid or mismatched Voyager identity certificate/key")
	}
	ca, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	key, ok := pair.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, errors.New("unsupported Voyager identity key")
	}
	if !ca.IsCA || !ca.BasicConstraintsValid || ca.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, errors.New("Voyager identity must be a certificate-signing authority")
	}
	if err := ca.CheckSignatureFrom(ca); err != nil {
		return nil, errors.New("Voyager identity must be self-signed")
	}
	public, _ := x509.MarshalPKIXPublicKey(key.Public())
	expected, _ := x509.MarshalPKIXPublicKey(ca.PublicKey)
	if !bytes.Equal(public, expected) || name == "" {
		return nil, errors.New("invalid Voyager identity")
	}
	issuer := &Issuer{ca: ca, key: key, name: name}
	if _, err := issuer.certificate(time.Now()); err != nil {
		return nil, err
	}
	return issuer, nil
}

func (i *Issuer) certificate(now time.Time) (*tls.Certificate, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if now.Before(i.ca.NotBefore) || !now.Before(i.ca.NotAfter) {
		return nil, errors.New("Voyager identity trust anchor expired or not yet valid")
	}
	if i.current != nil && now.Before(i.current.Leaf.NotAfter.Add(-48*time.Hour)) && !now.Before(i.current.Leaf.NotBefore) {
		return i.current, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	until := now.Add(7 * 24 * time.Hour)
	if until.After(i.ca.NotAfter) {
		until = i.ca.NotAfter
	}
	tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: i.name}, DNSNames: []string{i.name}, NotBefore: now.Add(-5 * time.Minute), NotAfter: until, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, i.ca, key.Public(), i.key)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	i.current = &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
	return i.current, nil
}

// TLSConfig renews on handshakes, even in a continuously running instance.
// No network call, local timer service or shared writable storage is needed.
func (i *Issuer) TLSConfig() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return i.certificate(time.Now()) }}
}
