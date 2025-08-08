package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/wweir/contatto/config"
)

// CommandRequest represents a unified command execution request
type CommandRequest struct {
	Command     string
	RawImage    *config.ImagePattern
	MirrorImage *config.ImagePattern
	Reason      string
	Source      string // "404_handler" or "version_checker"
	Done        chan struct{}
}

// CommandExecutor handles unified command execution via channels
type CommandExecutor struct {
	commandQueue chan CommandRequest
	config       *config.ConfigStruct
	enabled      bool
}

// NewCommandExecutor creates a new command executor
func NewCommandExecutor(config *config.ConfigStruct, queueSize int) *CommandExecutor {
	ce := &CommandExecutor{
		commandQueue: make(chan CommandRequest, queueSize),
		config:       config,
		enabled:      true,
	}

	// two image copy threads
	go ce.processCommandQueue()
	go ce.processCommandQueue()

	return ce
}

// ExecuteCommand queues a command for execution
func (ce *CommandExecutor) ExecuteCommand(req CommandRequest) {
	if !ce.enabled {
		if req.Done != nil {
			close(req.Done)
		}
		return
	}

	// Initialize Done channel if not provided
	if req.Done == nil {
		req.Done = make(chan struct{})
	}

	select {
	case ce.commandQueue <- req:
		slog.Info("queued command execution",
			"source", req.Source,
			"image", req.RawImage.String(),
			"reason", req.Reason)
	default:
		slog.Warn("command queue full, dropping request",
			"source", req.Source,
			"image", req.RawImage.String())
		if req.Done != nil {
			close(req.Done)
		}
	}
}

// processCommandQueue handles queued command requests
func (ce *CommandExecutor) processCommandQueue() {
	for cmdReq := range ce.commandQueue {
		err := ce.executeCommandInternal(cmdReq)
		if err != nil {
			slog.Error("failed to execute command",
				"error", err,
				"source", cmdReq.Source,
				"image", cmdReq.RawImage.String())
		}

		// Close the Done channel to signal completion
		if cmdReq.Done != nil {
			close(cmdReq.Done)
		}
	}
}

// executeCommandInternal performs the actual command execution
func (ce *CommandExecutor) executeCommandInternal(cmdReq CommandRequest) error {
	slog.Info("executing command",
		"source", cmdReq.Source,
		"raw", cmdReq.RawImage.String(),
		"mirror", cmdReq.MirrorImage.String(),
		"reason", cmdReq.Reason,
		"cmd", cmdReq.Command)

	startTime := time.Now()

	// Use a timeout for the command execution
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdReq.Command)
	out, err := cmd.CombinedOutput()

	if err != nil {
		slog.Error("command execution failed",
			"error", err,
			"output", string(out),
			"source", cmdReq.Source,
			"image", cmdReq.RawImage.String(),
			"duration", time.Since(startTime))
		return fmt.Errorf("command failed: %w", err)
	} else {
		slog.Info("command execution completed successfully",
			"source", cmdReq.Source,
			"image", cmdReq.RawImage.String(),
			"duration", time.Since(startTime))
	}

	return nil
}

// SetEnabled enables or disables command execution
func (ce *CommandExecutor) SetEnabled(enabled bool) {
	ce.enabled = enabled
	slog.Info("command execution", "enabled", enabled)
}

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
	mirror          *Mirror
	config          *config.ConfigStruct
	commandExecutor *CommandExecutor
	enabled         bool
}

// NewVersionChecker creates a new version checker
func NewVersionChecker(mirror *Mirror, config *config.ConfigStruct, commandExecutor *CommandExecutor) *VersionChecker {
	// Default configuration
	enabled := true

	// Apply user configuration if provided
	if config.VersionCheck != nil {
		enabled = config.VersionCheck.Enabled
	}

	vc := &VersionChecker{
		mirror:          mirror,
		config:          config,
		commandExecutor: commandExecutor,
		enabled:         enabled,
	}

	slog.Info("version checker initialized", "enabled", enabled)

	return vc
}

// CheckVersionConsistency checks if raw and mirror version are consistent for latest tags
func (vc *VersionChecker) CheckVersionConsistency(ctx context.Context, rawImage, mirrorImage *config.ImagePattern) {
	if !vc.enabled || strings.ToLower(rawImage.Tag) != "latest" {
		return
	}

	slog.Debug("performing version check", "raw", rawImage.String(), "mirror", mirrorImage.String())

	// Get manifests from both registries
	rawManifest, err := vc.getManifest(ctx, rawImage)
	if err != nil {
		slog.Error("failed to get raw manifest", "error", err, "image", rawImage.String())
		return
	}

	mirrorManifest, err := vc.getManifest(ctx, mirrorImage)
	if err != nil {
		slog.Warn("failed to get mirror manifest, assuming outdated", "error", err, "mirror", mirrorImage.String())
		vc.triggerUpdate(rawImage, mirrorImage, "mirror_manifest_unavailable")
		return
	}

	// Compare manifests by config digest
	if rawManifest.Config.Digest != mirrorManifest.Config.Digest {
		slog.Info("version mismatch detected",
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

// triggerUpdate queues an update request for the image using the unified command executor
func (vc *VersionChecker) triggerUpdate(rawImage, mirrorImage *config.ImagePattern, reason string) {
	// Find the rule for this registry
	rule, ok := vc.config.Rule[rawImage.Registry]
	if !ok {
		slog.Error("no rule found for registry", "registry", rawImage.Registry)
		return
	}

	if rule.OnMissingTpl == "" {
		slog.Warn("no update command configured", "registry", rawImage.Registry)
		return
	}

	cmdline, err := rule.RenderOnMissingCmd(map[string]any{
		"Raw":    rawImage,
		"Mirror": mirrorImage,
		"raw":    rawImage.String(),
		"mirror": mirrorImage.String(),
		"reason": reason,
	})
	if err != nil {
		slog.Error("failed to render update command", "error", err, "registry", rawImage.Registry)
		return
	}

	if cmdline == "" {
		return
	}

	// Use unified command executor
	cmdReq := CommandRequest{
		Command:     cmdline,
		RawImage:    rawImage,
		MirrorImage: mirrorImage,
		Reason:      reason,
		Source:      "version_checker",
		Done:        make(chan struct{}),
	}

	vc.commandExecutor.ExecuteCommand(cmdReq)
}

// SetEnabled enables or disables version checking
func (vc *VersionChecker) SetEnabled(enabled bool) {
	vc.enabled = enabled
	slog.Info("version checking", "enabled", enabled)
}
