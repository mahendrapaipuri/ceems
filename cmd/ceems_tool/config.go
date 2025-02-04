package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	config_util "github.com/prometheus/common/config"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const (
	charset           = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	basicAuthUsername = "ceems"
	certFile          = "cert.pem"
	keyFile           = "key.pem"
)

// The following structs are workaround until the upstream
// exporter toolkit fixes the MarshalYAML receivers on TLSConfig
// struct fields. We need only basic stuff here as exporter toolkit
// adds rest of the config some sane defaults to start with!
// Ref: https://github.com/prometheus/exporter-toolkit/pull/288

// TLSConfig is the config struct for TLS.
type TLSConfig struct {
	TLSCertPath string `yaml:"cert_file"`
	TLSKeyPath  string `yaml:"key_file"`
}

// WebConfig is the config struct for web.
type WebConfig struct {
	TLSConfig TLSConfig                     `yaml:"tls_server_config"`
	Users     map[string]config_util.Secret `yaml:"basic_auth_users"`
}

// GenerateWebConfig generates web config file.
func GenerateWebConfig(basicAuth bool, tls bool, hosts []string, validity time.Duration, outDir string) error {
	var password string

	var err error

	// Instantiate a config struct
	config := WebConfig{}

	// Generate basic config
	if basicAuth {
		config.Users, password, err = basicAuthConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error creating basic auth config:", err)

			return err
		}
	}

	// Generate TLS config
	if tls {
		config.TLSConfig, err = tlsConfig(hosts, validity, outDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error creating tls config:", err)

			return err
		}
	}

	// To expose secret as such without hiding it.
	config_util.MarshalSecretValue = true

	// Encode to YAML with indent set to 2
	var b bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&b)
	yamlEncoder.SetIndent(2)

	if err := yamlEncoder.Encode(&config); err != nil {
		fmt.Fprintln(os.Stderr, "error encoding web config", err)

		return err
	}

	// Write to disk
	configFile := filepath.Join(outDir, "web-config.yml")
	if err := os.WriteFile(configFile, b.Bytes(), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "failed to write web config file:", err)

		return err
	}

	fmt.Fprintln(os.Stderr, "web config file created at", configFile)
	fmt.Fprintln(os.Stderr, "plain text password for basic auth is", password)
	fmt.Fprintln(os.Stderr, "store the plain text password securely as you will need it to configure Prometheus")

	return nil
}

// tlsConfig returns a TLS config based on self signed TLS certificates.
func tlsConfig(hosts []string, validity time.Duration, outDir string) (TLSConfig, error) {
	// Make directory to store certificate files
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "error creating output directory:", err)

		return TLSConfig{}, err
	}

	// Generate self signed certificates
	// Nicked from https://go.dev/src/crypto/tls/generate_cert.go
	if err := selfSignedTLS(hosts, validity, outDir); err != nil {
		fmt.Fprintln(os.Stderr, "error generating self signed TLS certificate", err)

		return TLSConfig{}, err
	}

	// Setup TLS config
	config := TLSConfig{
		TLSCertPath: certFile,
		TLSKeyPath:  keyFile,
	}

	return config, nil
}

// publicKey returns the type of key based on private key.
func publicKey(priv any) any {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		if v, ok := k.Public().(ed25519.PublicKey); ok {
			return v
		}

		return nil
	default:
		return nil
	}
}

// selfSignedTLS creates a self signed certificates pair and writes to disk.
func selfSignedTLS(hosts []string, validity time.Duration, outDir string) error {
	// Generate private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	// ECDSA, ED25519 and RSA subject keys should have the DigitalSignature
	// KeyUsage bits set in the x509.Certificate template
	keyUsage := x509.KeyUsageDigitalSignature

	// Validity times
	notBefore := time.Now()
	notAfter := notBefore.Add(validity)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)

	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              keyUsage,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	// Use it as CA as well
	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageCertSign

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		return err
	}

	// Create file for certificate
	certOut, err := os.Create(filepath.Join(outDir, certFile))
	if err != nil {
		return err
	}

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}

	if err := certOut.Close(); err != nil {
		return err
	}

	keyOut, err := os.OpenFile(filepath.Join(outDir, keyFile), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return err
	}

	if err := keyOut.Close(); err != nil {
		return err
	}

	return nil
}

// basiAuthConfig returns a basic auth config with username and password.
func basicAuthConfig() (map[string]config_util.Secret, string, error) {
	// Generate a secret to be used as basic auth password
	passwordString, err := generateSecret(24)
	if err != nil {
		return nil, "", err
	}

	// Hash generated password
	hashedPassword, err := hashPassword(passwordString)
	if err != nil {
		return nil, "", err
	}

	return map[string]config_util.Secret{basicAuthUsername: config_util.Secret(hashedPassword)}, passwordString, nil
}

// hashPassword hashes the password using bcrypt.
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)

	return string(bytes), err
}

// generateSecret generates a secret based on provided length.
func generateSecret(length int) (string, error) {
	password := make([]byte, length)
	charsetLength := big.NewInt(int64(len(charset)))

	for i := range password {
		index, err := rand.Int(rand.Reader, charsetLength)
		if err != nil {
			return "", fmt.Errorf("error generating random index: %w", err)
		}

		password[i] = charset[index.Int64()]
	}

	return string(password), nil
}
