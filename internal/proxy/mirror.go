package proxy

import (
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/wweir/contatto/config"
)

// Mirror provides optimized routing and parsing for Docker registry paths
type Mirror struct {
	// Pre-compiled regex patterns for different endpoints
	manifestPattern *regexp.Regexp
	blobPattern     *regexp.Regexp

	// Cache for frequently accessed configurations
	configCache sync.Map // map[string]*CachedConfig

	// HTTP client with optimized settings
	httpClient *http.Client
}

// CachedConfig holds cached configuration data for a registry
type CachedConfig struct {
	Source    *config.Source
	CachedAt  time.Time
	TTL       time.Duration
}

// RouteInfo contains parsed route information
type RouteInfo struct {
	Endpoint string // "manifests", "blobs", etc.
	Project  string
	Repo     string
	Tag      string
	Valid    bool
}

// NewMirror creates an optimized route parser
func NewMirror(cfg *config.ConfigStruct) *Mirror {
	return &Mirror{
		// Pre-compile regex patterns for better performance
		manifestPattern: regexp.MustCompile(`^/v2/([^/]+)/([^/]+)/manifests/([^/]+)$`),
		blobPattern:     regexp.MustCompile(`^/v2/([^/]+)/([^/]+)/blobs/([^/]+)$`),

		// Optimized HTTP client with connection pooling
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
			},
		},
	}
}

// ParseRoute efficiently parses Docker registry API paths using pre-compiled regex
func (m *Mirror) ParseRoute(path string) RouteInfo {
	// Try manifest pattern first (more common)
	if matches := m.manifestPattern.FindStringSubmatch(path); matches != nil {
		return RouteInfo{
			Endpoint: "manifests",
			Project:  matches[1],
			Repo:     matches[2],
			Tag:      matches[3],
			Valid:    true,
		}
	}

	// Try blob pattern
	if matches := m.blobPattern.FindStringSubmatch(path); matches != nil {
		return RouteInfo{
			Endpoint: "blobs",
			Project:  matches[1],
			Repo:     matches[2],
			Tag:      matches[3],
			Valid:    true,
		}
	}

	// Handle /v2/ ping endpoint
	if path == "/v2/" {
		return RouteInfo{
			Endpoint: "ping",
			Valid:    true,
		}
	}

	return RouteInfo{Valid: false}
}

// GetCachedConfig retrieves cached configuration or loads it fresh
func (m *Mirror) GetCachedConfig(host string, config *config.ConfigStruct) *CachedConfig {
	// Try to get from cache first
	if cached, ok := m.configCache.Load(host); ok {
		cachedConfig := cached.(*CachedConfig)
		// Check if cache is still valid
		if time.Since(cachedConfig.CachedAt) < cachedConfig.TTL {
			return cachedConfig
		}
		// Cache expired, remove it
		m.configCache.Delete(host)
	}

	// Load fresh configuration
	newConfig := &CachedConfig{
		CachedAt: time.Now(),
		TTL:      5 * time.Minute, // Cache for 5 minutes
	}

	// Get source configuration
	if src, ok := config.Source[host]; ok {
		newConfig.Source = src
	}

	// Cache the new configuration
	m.configCache.Store(host, newConfig)

	return newConfig
}

// Rewrite provides optimized request rewriting logic
func (m *Mirror) Rewrite(r *http.Request, cfg *config.ConfigStruct) (*config.ImagePattern, *config.ImagePattern, error) {
	// Get host from query parameter
	query := r.URL.Query()
	host := query.Get("ns")
	if host == "" {
		host = "docker.io"
	}

	// Parse route information efficiently
	routeInfo := m.ParseRoute(r.URL.Path)
	if !routeInfo.Valid {
		return nil, nil, nil
	}

	// Skip processing for ping endpoint
	if routeInfo.Endpoint == "ping" {
		return nil, nil, nil
	}

	// Get cached configuration
	cachedConfig := m.GetCachedConfig(host, cfg)

	// Check if we have a source config for this host
	if cachedConfig.Source == nil {
		// No mapping rule, direct forward
		return nil, nil, nil
	}

	// Create source image pattern
	srcImage := &config.ImagePattern{
		Registry: host,
		Project:  routeInfo.Project,
		Repo:     routeInfo.Repo,
		Tag:      routeInfo.Tag,
	}
	srcImage.SetInsecure(cachedConfig.Source.Insecure)

	// Validate source image
	if err := srcImage.Validate(); err != nil {
		return nil, nil, err
	}

	// Render mirror path
	mirrorPath, err := cachedConfig.Source.RenderMirrorPath(srcImage.Project, srcImage.Repo, srcImage.Tag)
	if err != nil {
		return nil, nil, err
	}

	// Create destination image pattern
	dstImage := &config.ImagePattern{Registry: cfg.Mirror.Registry}
	if err := dstImage.ParseImage(cfg.Mirror.Registry + "/" + mirrorPath); err != nil {
		return nil, nil, err
	}
	dstImage.SetInsecure(cfg.Mirror.Insecure)

	return srcImage, dstImage, nil
}

// PrewarmCache preloads frequently used configurations into cache
func (m *Mirror) PrewarmCache(cfg *config.ConfigStruct) {
	// Common registries to pre-cache
	commonRegistries := []string{"docker.io", "quay.io", "gcr.io", "registry.k8s.io"}

	for _, host := range commonRegistries {
		m.GetCachedConfig(host, cfg)
	}
}

// GetHTTPClient returns the optimized HTTP client
func (m *Mirror) GetHTTPClient() *http.Client {
	return m.httpClient
}
