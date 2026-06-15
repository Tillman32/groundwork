package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Verifier struct {
	trustedPublishers map[string]string // publisherID -> public key
}

func NewVerifier(trustedPublishers map[string]string) *Verifier {
	return &Verifier{
		trustedPublishers: trustedPublishers,
	}
}

type ArtifactInfo struct {
	PluginName    string
	PluginVersion string
	Platform      string
	Arch          string
	Path          string
	SHA256        string
	Signature     string
	PublisherID   string
}

func (v *Verifier) VerifyArtifact(info *ArtifactInfo) error {
	// Check publisher is trusted
	if _, ok := v.trustedPublishers[info.PublisherID]; !ok {
		return fmt.Errorf("publisher %s is not trusted", info.PublisherID)
	}

	// Verify SHA256
	if err := v.verifyDigest(info.Path, info.SHA256); err != nil {
		return fmt.Errorf("digest verification failed: %w", err)
	}

	// Verify signature
	if err := v.verifySignature(info.Path, info.Signature, v.trustedPublishers[info.PublisherID]); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

func (v *Verifier) verifyDigest(path, expectedSHA256 string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open artifact: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedSHA256 {
		return fmt.Errorf("digest mismatch: expected %s, got %s", expectedSHA256, actual)
	}
	return nil
}

func (v *Verifier) verifySignature(artifactPath, signature, publicKey string) error {
	// In production, use cosign for verification
	_ = artifactPath
	_ = signature
	_ = publicKey
	return nil // Placeholder
}

func ComputeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ArtifactDir(dataDir, pluginName, pluginVersion, platform, arch string) string {
	return filepath.Join(dataDir, "artifacts", pluginName, pluginVersion, platform, arch)
}

func EnsureArtifactDir(dataDir, pluginName, pluginVersion, platform, arch string) (string, error) {
	dir := ArtifactDir(dataDir, pluginName, pluginVersion, platform, arch)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}