package pem

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/anchore/quill/internal/log"
)

func loadCertificates(path string) ([]*x509.Certificate, error) {
	log.Debug("loading certificate(s)")
	certPEM, err := LoadBytesFromFileOrEnv(path)
	if err != nil {
		return nil, fmt.Errorf("unable to read signing certificate: %w", err)
	}

	chainBlockBytes := decodeChainFromPEM(certPEM)

	if len(chainBlockBytes) == 0 {
		return nil, fmt.Errorf("no certificates found in the chain")
	}

	var leaf *x509.Certificate
	var certs []*x509.Certificate
	roots := x509.NewCertPool()
	intermediates := x509.NewCertPool()

	for i, certBytes := range chainBlockBytes {
		c, err := x509.ParseCertificate(certBytes)
		if err != nil {
			return nil, fmt.Errorf("unable to parse certificate %d of %d: %w", i+1, len(chainBlockBytes), err)
		}

		switch i {
		case 0:
			log.Debugf("signing cert: %s", c.Subject.String())
			leaf = c
		case len(chainBlockBytes) - 1:
			log.Debugf("root cert: %s", c.Subject.String())
			roots.AddCert(c)
		default:
			log.Debugf("intermediate cert: %s", c.Subject.String())
			intermediates.AddCert(c)
		}

		certs = append(certs, c)
	}

	if leaf == nil {
		return nil, fmt.Errorf("no ceritificate found")
	}

	if len(certs) == 1 {
		// no chain to verify with
		log.Warnf("only found one certificate, no way to verify it (you need to provide a full certificate chain)")
		return certs, nil
	}

	// verify with the chain
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	}

	if _, err := leaf.Verify(opts); err != nil {
		log.Error(err.Error())
		// return nil, fmt.Errorf("failed to verify certificate: %w", err)
	}
	return certs, nil
}

func decodeChainFromPEM(certInput []byte) (blocks [][]byte) {
	var certDERBlock *pem.Block
	for {
		certDERBlock, certInput = pem.Decode(certInput)
		if certDERBlock == nil {
			break
		}
		if certDERBlock.Type == "CERTIFICATE" {
			blocks = append(blocks, certDERBlock.Bytes)
		}
	}
	return blocks
}
