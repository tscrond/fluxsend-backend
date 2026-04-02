package cdn

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"
	"github.com/tscrond/fluxsend-backend/pkg"
)

type CloudFrontURLSigner struct {
	baseBucketName string
	domain         string
	keyPairID      string
	privateKey     *rsa.PrivateKey
}

func NewCloudFrontURLSigner(baseBucketName, domain, keyPairID, privateKeyPath string) (*CloudFrontURLSigner, error) {
	domain = strings.TrimSpace(domain)
	keyPairID = strings.TrimSpace(keyPairID)
	privateKeyPath = strings.TrimSpace(privateKeyPath)

	if baseBucketName == "" {
		return nil, fmt.Errorf("missing base bucket name for CloudFront signer")
	}
	if domain == "" {
		return nil, fmt.Errorf("missing CLOUDFRONT_DOMAIN")
	}
	if keyPairID == "" {
		return nil, fmt.Errorf("missing CLOUDFRONT_KEY_PAIR_ID")
	}
	if privateKeyPath == "" {
		return nil, fmt.Errorf("missing CLOUDFRONT_PRIVATE_KEY_PATH")
	}

	normalizedDomain, err := normalizeCloudFrontDomain(domain)
	if err != nil {
		return nil, err
	}

	privateKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}

	return &CloudFrontURLSigner{
		baseBucketName: baseBucketName,
		domain:         normalizedDomain,
		keyPairID:      keyPairID,
		privateKey:     privateKey,
	}, nil
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("invalid PEM")
	}

	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	privKey, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not RSA key")
	}

	return privKey, nil
}

func (s *CloudFrontURLSigner) SignURL(bucket, object string, expiresAt time.Time) (string, error) {
	userID := pkg.ExtractUserIdFromBucketName(s.baseBucketName, bucket)
	if userID == "" {
		return "", fmt.Errorf("cannot derive user ID from bucket %q for CloudFront signing", bucket)
	}

	resourceURL := url.URL{
		Scheme: "https",
		Host:   s.domain,
		Path:   "/" + userID + "/" + strings.TrimPrefix(object, "/"),
	}

	log.Println("FINAL PATH:", resourceURL.String())

	signer := sign.NewURLSigner(s.keyPairID, s.privateKey)

	policy := &sign.Policy{
		Statements: []sign.Statement{
			{
				Resource: resourceURL.String(),
				Condition: sign.Condition{
					DateLessThan: &sign.AWSEpochTime{Time: expiresAt},
				},
			},
		},
	}

	signedURL, err := signer.SignWithPolicy(resourceURL.String(), policy)

	log.Println(signedURL)
	if err != nil {
		return "", fmt.Errorf("failed to sign CloudFront URL: %w", err)
	}

	return signedURL, nil
}

func normalizeCloudFrontDomain(domain string) (string, error) {
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		parsed, err := url.Parse(domain)
		if err != nil {
			return "", fmt.Errorf("invalid CLOUDFRONT_DOMAIN: %w", err)
		}
		if parsed.Host == "" {
			return "", fmt.Errorf("invalid CLOUDFRONT_DOMAIN: missing host")
		}
		if parsed.Path != "" && parsed.Path != "/" {
			return "", fmt.Errorf("invalid CLOUDFRONT_DOMAIN: path is not allowed")
		}
		return parsed.Host, nil
	}

	if strings.Contains(domain, "/") {
		return "", fmt.Errorf("invalid CLOUDFRONT_DOMAIN: path is not allowed")
	}

	return domain, nil
}
