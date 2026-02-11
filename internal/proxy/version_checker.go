package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/wweir/contatto/config"
)

// ManifestInfo holds Docker manifest information for version comparison
type ManifestInfo struct {
	MediaType     string    `json:"mediaType"`
	SchemaVersion int       `json:"schemaVersion"`
	Digest        string    `json:"digest"`
	Config        ConfigRef `json:"config"`
}

type ConfigRef struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// VersionChecker handles version consistency checks for latest tags
type VersionChecker struct {
	mirror      *Mirror
	config      *config.ConfigStruct
	imageCopier *ImageCopier
	enabled     bool
}

// NewVersionChecker creates a new version checker
func NewVersionChecker(mirror *Mirror, config *config.ConfigStruct, imageCopier *ImageCopier) *VersionChecker {
	vc := &VersionChecker{
		mirror:      mirror,
		config:      config,
		imageCopier: imageCopier,
		enabled:     true, // Always enabled per new configuration
	}

	slog.Info("version checker initialized", "enabled", vc.enabled)

	return vc
}

// CheckVersionConsistency checks if raw and mirror version are consistent for all tags
func (vc *VersionChecker) CheckVersionConsistency(ctx context.Context, rawImage, mirrorImage *config.ImagePattern) {
	if !vc.enabled || rawImage.Tag == "" {
		return
	}

	slog.Debug("performing version check", "raw", rawImage.String(), "mirror", mirrorImage.String())

	// Get manifest from source registry
	rawManifest, err := vc.getManifest(ctx, rawImage)
	if err != nil {
		slog.Error("failed to get raw manifest", "error", err, "image", rawImage.String())
		return
	}

	// Get manifest from mirror registry
	mirrorManifest, err := vc.getManifest(ctx, mirrorImage)
	if err != nil {
		slog.Warn("mirror image not found, will copy", "error", err, "mirror", mirrorImage.String())
		vc.triggerUpdate(rawImage, mirrorImage, "mirror_image_not_found")
		return
	}

	// Compare manifests by config digest
	if rawManifest.Config.Digest != mirrorManifest.Config.Digest {
		slog.Info("version mismatch detected, will copy",
			"raw_digest", rawManifest.Config.Digest,
			"mirror_digest", mirrorManifest.Config.Digest,
			"image", rawImage.String())
		vc.triggerUpdate(rawImage, mirrorImage, "version_mismatch")
	} else {
		slog.Debug("versions are consistent", "image", rawImage.String())
	}
}

// getManifest retrieves manifest information from a registry
func (vc *VersionChecker) getManifest(ctx context.Context, image *config.ImagePattern) (*ManifestInfo, error) {
	// Build manifest URL
	manifestURL := fmt.Sprintf("%s://%s/v2/%s/%s/manifests/%s",
		image.Scheme, image.Registry, image.Project, image.Repo, image.Tag)

	// Create request with proper headers
	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, err
	}

	// Set Accept header for Docker manifest v2
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Set("User-Agent", "contatto-proxy/1.0")

	// Use mirror's HTTP client for connection pooling
	resp, err := vc.mirror.GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse manifest
	var manifest ManifestInfo
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	// Get digest from response header if available
	if digest := resp.Header.Get("Docker-Content-Digest"); digest != "" {
		manifest.Digest = digest
	}

	return &manifest, nil
}

// triggerUpdate queues an update request for the image using the image copier
func (vc *VersionChecker) triggerUpdate(rawImage, mirrorImage *config.ImagePattern, reason string) {
	// Use image copier directly instead of external command
	done, err := vc.imageCopier.QueueCopy(rawImage, mirrorImage)
	if err != nil {
		slog.Error("failed to queue image copy", "error", err, "image", rawImage.String())
		return
	}

	slog.Info("image copy queued",
		"source", "version_checker",
		"image", rawImage.String(),
		"reason", reason)

	// Wait for copy to complete (async)
	go func() {
		<-done
		slog.Info("image copy completed", "image", rawImage.String())
	}()
}

// SetEnabled enables or disables version checking
func (vc *VersionChecker) SetEnabled(enabled bool) {
	vc.enabled = enabled
	slog.Info("version checking", "enabled", enabled)
}
