package extract

import (
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/github/smimesign/ietf-cms/oid"

	"github.com/github/smimesign/ietf-cms/protocol"

	"github.com/anchore/quill/pkg/macho"
	cms "github.com/github/smimesign/ietf-cms"
)

type SignatureDetails struct {
	Blob          BlobDetails          `json:"blob"`
	Base64        string               `json:"base64"`
	CMSValidation CMSValidationDetails `json:"cmsValidation"`
	Certificates  []Certificate        `json:"certificates"`
	Signers       []Signer             `json:"signers"`
}

type Certificate struct {
	PEM    string            `json:"pem"`
	Parsed *x509.Certificate `json:"parsed"`
}

type Signer struct {
	ID               string             `json:"id"`
	Signature        AlgorithmWithValue `json:"signature"`
	SignedAttributes []Attribute        `json:"signedAttributes"`
	DigestAlgorithm  Algorithm          `json:"digestAlgorithm"`
}

type Attribute struct {
	OID    string `json:"oid"`
	Base64 string `json:"base64"`
}

type Algorithm struct {
	AlgorithmOID     string `json:"algorithmOID"`
	Base64Parameters string `json:"base64Parameters"`
}

type AlgorithmWithValue struct {
	Base64 string `json:"base64"`
	Algorithm
}

type CMSValidationDetails struct {
	IsValid              bool                    `json:"isValid"`
	ErrorMessage         string                  `json:"errorMessage"`
	VerifiedCertificates [][][]*x509.Certificate `json:"verifiedCertificates"`
}

func getSignatures(m file) []SignatureDetails {
	b, err := m.internalFile.CMSBlobBytes(macho.SigningOrder)
	if err != nil {
		// TODO
		panic(err)
	}

	hashObj := crypto.SHA256
	hasher := hashObj.New()
	hasher.Write(b)
	hash := hasher.Sum(nil)

	superBlob := m.blacktopFile.CodeSignature()

	ci, err := protocol.ParseContentInfo(superBlob.CMSSignature)
	if err != nil {
		// TODO
		panic(err)
	}

	sd, err := cms.ParseSignedData(superBlob.CMSSignature)
	if err != nil {
		// TODO
		panic(err)
	}

	psd, err := ci.SignedDataContent()
	if err != nil {
		// TODO
		panic(err)
	}

	// TODO: support multiple CDs
	cdBytes, err := m.internalFile.CDBytes(macho.SigningOrder, 0)
	if err != nil {
		// TODO
		panic(err)
	}

	// TODO: allow for specifying a root of trust

	// TODO: add verify options
	verifiedCerts, cmsErr := sd.VerifyDetached(cdBytes, x509.VerifyOptions{})
	cmsValid := cmsErr == nil
	var cmsErrorStr string
	if cmsErr != nil {
		cmsErrorStr = cmsErr.Error()
	}

	parsedCerts, err := psd.X509Certificates()
	if err != nil {
		// TODO
		panic(err)
	}

	var certs []Certificate
	for idx, cert := range parsedCerts {
		certs = append(certs, Certificate{
			PEM:    base64.StdEncoding.EncodeToString(psd.Certificates[idx].Bytes),
			Parsed: cert,
		})
	}

	var signers []Signer
	for _, s := range psd.SignerInfos {
		var atts []Attribute
		for _, att := range s.SignedAttrs {
			atts = append(atts, Attribute{
				OID:    att.Type.String(),
				Base64: base64.StdEncoding.EncodeToString(att.RawValue.Bytes),
			})
		}
		signers = append(signers, Signer{
			ID: string(s.SID.Bytes),
			Signature: AlgorithmWithValue{
				Base64: base64.StdEncoding.EncodeToString(s.Signature),
				Algorithm: Algorithm{
					AlgorithmOID:     s.SignatureAlgorithm.Algorithm.String(),
					Base64Parameters: base64.StdEncoding.EncodeToString(s.SignatureAlgorithm.Parameters.Bytes),
				},
			},
			SignedAttributes: atts,
			DigestAlgorithm: Algorithm{
				AlgorithmOID:     s.DigestAlgorithm.Algorithm.String(),
				Base64Parameters: base64.StdEncoding.EncodeToString(s.DigestAlgorithm.Parameters.Bytes),
			},
		})
	}

	return []SignatureDetails{
		{
			Blob: BlobDetails{
				Base64: base64.StdEncoding.EncodeToString(b),
				Digest: Digest{
					Algorithm: algorithmName(hashObj),
					Value:     hex.EncodeToString(hash),
				},
			},
			Base64: base64.StdEncoding.EncodeToString(superBlob.CMSSignature),
			CMSValidation: CMSValidationDetails{
				IsValid:              cmsValid,
				ErrorMessage:         cmsErrorStr,
				VerifiedCertificates: verifiedCerts,
			},
			Certificates: certs,
			Signers:      signers,
		},
	}
}

func (a Attribute) String() string {
	var oidHint string
	switch a.OID {
	case oid.AttributeSigningTime.String():
		oidHint = "(signing timestamp)"
	case oid.AttributeMessageDigest.String():
		oidHint = "(message digest)"
	case oid.AttributeContentType.String():
		oidHint = "(content type)"
	}
	return tprintf(
		`OID:        {{.OID}} {{.OIDHint}}
Base64:     {{.Base64}}
`,
		struct {
			Attribute
			OIDHint string
		}{
			Attribute: a,
			OIDHint:   oidHint,
		},
	)
}

func (av AlgorithmWithValue) String() string {
	return tprintf(
		`Algorithm: {{.AlgorithmOID}}
Base64:    {{.Base64}}
`,
		struct {
			AlgorithmWithValue
		}{
			AlgorithmWithValue: av,
		},
	)
}

func (s Signer) String() string {

	var atts []string
	for idx, a := range s.SignedAttributes {
		atts = append(atts, fmt.Sprintf("Attribute %d:\n%s", idx+1, doIndent(a.String(), "  ")))
	}

	return tprintf(
		`Signature: {{.FormattedSignature}}
{{.FormattedAttributes}}
`,
		struct {
			Signer
			FormattedAttributes string
			FormattedSignature  string
		}{
			Signer:              s,
			FormattedAttributes: strings.Join(atts, ""),
			FormattedSignature:  "\n" + strings.TrimRight(doIndent(s.Signature.String(), "  "), " \n"),
		},
	)
}

func addIfUsageSet(usageHints []string, val x509.KeyUsage, bit x509.KeyUsage, title string) []string {
	if val&bit != 0 {
		usageHints = append(usageHints, title)
	}
	return usageHints
}

func (c Certificate) String() string {
	var exts []string
	for _, ext := range c.Parsed.Extensions {
		exts = append(exts, ext.Id.String())
	}

	var usageHints []string
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageDigitalSignature, "digital signature")
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageCertSign, "cert sign")
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageContentCommitment, "content commitment")
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageDataEncipherment, "data encipherment")
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageCRLSign, "CRL sign")
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageDecipherOnly, "decipher only")
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageEncipherOnly, "encipher only")
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageKeyAgreement, "key agreement")
	usageHints = addIfUsageSet(usageHints, c.Parsed.KeyUsage, x509.KeyUsageKeyEncipherment, "key encipherment")

	var usageHint string
	if len(usageHints) > 0 {
		usageHint = fmt.Sprintf("(%s)", strings.Join(usageHints, ", "))
	}

	var usages []string
	for _, u := range c.Parsed.ExtKeyUsage {
		value := fmt.Sprintf("0x%x", u)
		var hint string
		switch u {
		case x509.ExtKeyUsageClientAuth:
			hint = "(client)"
		case x509.ExtKeyUsageAny:
			hint = "(any)"
		case x509.ExtKeyUsageCodeSigning:
			hint = "(code signing)"
		case x509.ExtKeyUsageEmailProtection:
			hint = "(email protection)"
		case x509.ExtKeyUsageIPSECEndSystem:
			hint = "(IPSEC end system)"
		case x509.ExtKeyUsageIPSECTunnel:
			hint = "(IPSEC tunnel)"
		case x509.ExtKeyUsageIPSECUser:
			hint = "(IPSEC user)"
		case x509.ExtKeyUsageMicrosoftCommercialCodeSigning:
			hint = "(microsoft commercial code signing)"
		case x509.ExtKeyUsageMicrosoftKernelCodeSigning:
			hint = "(microsoft kernel code signing)"
		case x509.ExtKeyUsageNetscapeServerGatedCrypto:
			hint = "(netscape server gated crypto)"
		case x509.ExtKeyUsageOCSPSigning:
			hint = "(OCSP signing)"
		case x509.ExtKeyUsageServerAuth:
			hint = "(server auth)"
		case x509.ExtKeyUsageTimeStamping:
			hint = "(timestamping)"
		}
		usages = append(usages, value+" "+hint)
	}

	usagesStr := "[]"
	if len(usages) > 0 {
		usagesStr = fmt.Sprintf("[\n%s\n]", doIndent(strings.Join(usages, ",\n"), "  "))
	}

	return tprintf(
		`Subject:
  CN:  {{.Parsed.Subject.CommonName}}
  OU:  {{.SOU}}
Issuer:
  CN:  {{.Parsed.Issuer.CommonName}}
  OU:  {{.IOU}}
KeyUsage:   {{.Usage}} {{.UsageHint}}
Extensions: {{.Extensions}}
ExtendedKeyUsage: {{.ExtendedUsage}}
`,
		struct {
			Certificate
			SOU           string
			IOU           string
			Usage         string
			UsageHint     string
			ExtendedUsage string
			Extensions    string
		}{
			Certificate:   c,
			SOU:           strings.Join(c.Parsed.Subject.OrganizationalUnit, ", "),
			IOU:           strings.Join(c.Parsed.Issuer.OrganizationalUnit, ", "),
			Usage:         fmt.Sprintf("0x%x", c.Parsed.KeyUsage),
			UsageHint:     usageHint,
			ExtendedUsage: usagesStr,
			Extensions:    strings.Join(exts, ", "),
		},
	)
}

func (s SignatureDetails) String() string {
	var validationError string
	if !s.CMSValidation.IsValid {
		validationError = fmt.Sprintf("(%s)", s.CMSValidation.ErrorMessage)
	}

	var certs []string
	for idx, c := range s.Certificates {
		certs = append(certs, fmt.Sprintf("Certificate %d:\n%s\n", idx+1, strings.TrimRight(doIndent(c.String(), "  "), " \n")))
	}

	var signers []string
	for idx, signer := range s.Signers {
		signers = append(signers, fmt.Sprintf("Signer %d:\n%s", idx+1, doIndent(signer.String(), "  ")))
	}

	return tprintf(
		`Valid: {{.CMSValidation.IsValid}} {{.ValidationError}}
{{.FormattedCerts}}
{{.FormattedSigners}}
`,
		struct {
			SignatureDetails
			ValidationError  string
			FormattedCerts   string
			CertificateCount string
			FormattedSigners string
			SignersCount     string
		}{
			SignatureDetails: s,
			ValidationError:  validationError,
			FormattedCerts:   strings.TrimRight(strings.Join(certs, ""), " \n"),
			FormattedSigners: strings.TrimRight(strings.Join(signers, ""), " \n"),
		},
	)
}
