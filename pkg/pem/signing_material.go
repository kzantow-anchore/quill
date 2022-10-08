package pem

import (
	"crypto"
	"crypto/x509"
	"fmt"
)

type SigningMaterial struct {
	Signer crypto.Signer
	Certs  []*x509.Certificate
}

func NewSigningMaterial(certFile, privateKeyPath, password string) (*SigningMaterial, error) {
	if certFile == "" || privateKeyPath == "" {
		return nil, nil
	}
	certs, err := loadCertificatesFromFile(certFile)
	if err != nil {
		return nil, err
	}

	privateKey, err := loadPrivateKey(privateKeyPath, password)
	if err != nil {
		return nil, err
	}

	signer, ok := privateKey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("unable to derive signer from private key")
	}

	return &SigningMaterial{
		Signer: signer,
		Certs:  certs,
	}, nil
}
