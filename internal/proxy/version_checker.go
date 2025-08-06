package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wweir/contatto/config"
)

// ManifestInfo holds Docker manifest information for version comparison
type ManifestInfo struct {
	MediaType     string    `json:"mediaType"`
	SchemaVersion int       `json:"schemaVersion"`
	Digest        string    `json:"digest"`
	Config        ConfigRef `json:"config"`
	LastChecked   time.Time `json:"-"`
}

type ConfigRef struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// VersionChecker handles version consistency checks for latest tags
type VersionChecker struct {
	mirror        *Mirror
	manifestCache sync.Map // map[string]*ManifestInfo
	updateQueue   chan UpdateRequest
	config        *config.ConfigStruct
	checkInterval time.Duration
	enabled       bool
}

type UpdateRequest struct {
	RawImage    *config.ImagePattern
	MirrorImage *config.ImagePattern
	Reason      string
}

// NewVersionChecker creates a new version checker
func NewVersionChecker(mirror *Mirror, config *config.ConfigStruct) *VersionChecker {
	// Default configuration
	enabled := true
	checkInterval := 5 * time.Minute
	queueSize := 100

	// Apply user configuration if provided
	if config.VersionCheck != nil {
		enabled = config.VersionCheck.Enabled
		if config.VersionCheck.CheckInterval != "" {
			if duration, err := time.ParseDuration(config.VersionCheck.CheckInterval); err == nil {
				checkInterval = duration
			} else {
				slog.Warn("invalid check_interval, using default", "interval", config.VersionCheck.CheckInterval, "default", checkInterval)
			}
		}
		if config.VersionCheck.MaxQueueSize > 0 {
			queueSize = config.VersionCheck.MaxQueueSize
		}
	}

	vc := &VersionChecker{
		mirror:        mirror,
		config:        config,
		updateQueue:   make(chan UpdateRequest, queueSize),
		checkInterval: checkInterval,
		enabled:       enabled,
	}

	slog.Info("version checker initialized",
		"enabled", enabled,
		"check_interval", checkInterval,
		"queue_size", queueSize)

	// Start background workers if enabled
	if enabled {
		go vc.processUpdateQueue()
	}

	return vc
}

// CheckVersionConsistency checks if raw and mirror versions are consistent for latest tags
func (vc *VersionChecker) CheckVersionConsistency(ctx context.Context, rawImage, mirrorImage *config.ImagePattern) {
	if !vc.enabled {
		return
	}

	// Only check for latest tags
	if !vc.shouldCheckVersion(rawImage) {
		return
	}

	// Perform async version check to not block main request
	go func() {
		if err := vc.performVersionCheck(ctx, rawImage, mirrorImage); err != nil {
			slog.Error("version check failed", "error", err, "image", rawImage.String())
		}
	}()
}

// shouldCheckVersion determines if we should check version for this image
func (vc *VersionChecker) shouldCheckVersion(image *config.ImagePattern) bool {
	// Check for latest tag (case insensitive)
	if strings.ToLower(image.Tag) != "latest" {
		return false
	}

	// Check if we've checked recently (rate limiting)
	cacheKey := image.Registry + "/" + image.Project + "/" + image.Repo
	if cached, ok := vc.manifestCache.Load(cacheKey); ok {
		manifest := cached.(*ManifestInfo)
		if time.Since(manifest.LastChecked) < vc.checkInterval {
			return false // Skip if checked recently
		}
	}

	return true
}

// performVersionCheck compares manifests between raw and mirror registries
func (vc *VersionChecker) performVersionCheck(ctx context.Context, rawImage, mirrorImage *config.ImagePattern) error {
	slog.Debug("performing version check", "raw", rawImage.String(), "mirror", mirrorImage.String())

	// Get manifests from both registries
	rawManifest, err := vc.getManifest(ctx, rawImage)
	if err != nil {
		return fmt.Errorf("failed to get raw manifest: %w", err)
	}

	mirrorManifest, err := vc.getManifest(ctx, mirrorImage)
	if err != nil {
		slog.Warn("failed to get mirror manifest, assuming outdated", "error", err, "mirror", mirrorImage.String())
		// If mirror manifest is unavailable, trigger update
		vc.triggerUpdate(rawImage, mirrorImage, "mirror_manifest_unavailable")
		return nil
	}

	// Compare manifests
	if !vc.manifestsMatch(rawManifest, mirrorManifest) {
		slog.Info("version mismatch detected",
			"raw_digest", rawManifest.Config.Digest,
			"mirror_digest", mirrorManifest.Config.Digest,
			"image", rawImage.String())

		vc.triggerUpdate(rawImage, mirrorImage, "version_mismatch")
	} else {
		slog.Debug("versions are consistent", "image", rawImage.String())
	}

	// Update cache
	cacheKey := rawImage.Registry + "/" + rawImage.Project + "/" + rawImage.Repo
	rawManifest.LastChecked = time.Now()
	vc.manifestCache.Store(cacheKey, rawManifest)

	return nil
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

// manifestsMatch compares two manifests to determine if they represent the same image version
func (vc *VersionChecker) manifestsMatch(raw, mirror *ManifestInfo) bool {
	// Primary comparison: config digest (most reliable)
	if raw.Config.Digest != "" && mirror.Config.Digest != "" {
		return raw.Config.Digest == mirror.Config.Digest
	}

	// Fallback: manifest digest
	if raw.Digest != "" && mirror.Digest != "" {
		return raw.Digest == mirror.Digest
	}

	// If we can't compare reliably, assume they don't match to be safe
	return false
}

// triggerUpdate queues an update request for the image
func (vc *VersionChecker) triggerUpdate(rawImage, mirrorImage *config.ImagePattern, reason string) {
	updateReq := UpdateRequest{
		RawImage:    rawImage,
		MirrorImage: mirrorImage,
		Reason:      reason,
	}

	select {
	case vc.updateQueue <- updateReq:
		slog.Info("queued image update", "image", rawImage.String(), "reason", reason)
	default:
		slog.Warn("update queue full, dropping request", "image", rawImage.String())
	}
}

// processUpdateQueue handles queued update requests
func (vc *VersionChecker) processUpdateQueue() {
	for updateReq := range vc.updateQueue {
		if err := vc.executeUpdate(updateReq); err != nil {
			slog.Error("failed to execute update", "error", err, "image", updateReq.RawImage.String())
		}
	}
}

// executeUpdate performs the actual image update
func (vc *VersionChecker) executeUpdate(updateReq UpdateRequest) error {
	slog.Info("executing image update",
		"raw", updateReq.RawImage.String(),
		"mirror", updateReq.MirrorImage.String(),
		"reason", updateReq.Reason)

	// Find the rule for this registry
	rule, ok := vc.config.Rule[updateReq.RawImage.Registry]
	if !ok {
		return fmt.Errorf("no rule found for registry: %s", updateReq.RawImage.Registry)
	}

	// Execute the on-missing command to trigger update
	if rule.OnMissingTpl == "" {
		slog.Warn("no update command configured", "registry", updateReq.RawImage.Registry)
		return nil
	}

	cmdline, err := rule.RenderOnMissingCmd(map[string]any{
		"Raw":    updateReq.RawImage,
		"Mirror": updateReq.MirrorImage,
		"raw":    updateReq.RawImage.String(),
		"mirror": updateReq.MirrorImage.String(),
		"reason": updateReq.Reason,
	})
	if err != nil {
		return fmt.Errorf("failed to render update command: %w", err)
	}

	if cmdline == "" {
		return nil
	}

	// Execute update command asynchronously
	go func() {
		startTime := time.Now()
		slog.Info("running image update command", "cmd", cmdline, "image", updateReq.RawImage.String())

		// Use a timeout for the update command
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, "sh", "-c", cmdline)
		out, err := cmd.CombinedOutput()

		if err != nil {
			slog.Error("image update command failed",
				"error", err,
				"output", string(out),
				"image", updateReq.RawImage.String(),
				"duration", time.Since(startTime))
		} else {
			slog.Info("image update completed successfully",
				"image", updateReq.RawImage.String(),
				"duration", time.Since(startTime))

			// Clear cache entry to force recheck on next request
			cacheKey := updateReq.RawImage.Registry + "/" + updateReq.RawImage.Project + "/" + updateReq.RawImage.Repo
			vc.manifestCache.Delete(cacheKey)
		}
	}()

	return nil
}

// SetEnabled enables or disables version checking
func (vc *VersionChecker) SetEnabled(enabled bool) {
	vc.enabled = enabled
	slog.Info("version checking", "enabled", enabled)
}

// SetCheckInterval sets the minimum interval between version checks for the same image
func (vc *VersionChecker) SetCheckInterval(interval time.Duration) {
	vc.checkInterval = interval
	slog.Info("version check interval updated", "interval", interval)
}
