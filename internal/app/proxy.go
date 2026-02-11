package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/julienschmidt/httprouter"
	"github.com/wweir/contatto/config"
	"github.com/wweir/contatto/internal/proxy"
)

type ProxyCmd struct {
	firstRequest   sync.Map
	mirror         *proxy.Mirror
	versionChecker *proxy.VersionChecker
	imageCopier    *proxy.ImageCopier
}

func (c *ProxyCmd) Run(cfg *config.ConfigStruct) error {
	slog.Info("Starting proxy", "version", config.Version, "date", config.Date, "config", cfg)

	// Initialize components
	c.mirror = proxy.NewMirror(cfg)
	c.imageCopier = proxy.NewImageCopier(cfg)
	c.versionChecker = proxy.NewVersionChecker(c.mirror, cfg, c.imageCopier)

	// Prewarm cache with common registries
	c.mirror.PrewarmCache(cfg)

	// Create httprouter
	router := httprouter.New()
	c.setupRoutes(cfg, router)

	return http.ListenAndServe(cfg.Addr, router)
}

func (c *ProxyCmd) setupRoutes(cfg *config.ConfigStruct, router *httprouter.Router) {
	router.GET("/v2/", c.handlePing)
	router.GET("/v2/:project/:repo/manifests/:reference", c.handleProxy(cfg))
	router.HEAD("/v2/:project/:repo/manifests/:reference", c.handleProxy(cfg))
	router.GET("/v2/:project/:repo/blobs/:digest", c.handleProxy(cfg))
	router.HEAD("/v2/:project/:repo/blobs/:digest", c.handleProxy(cfg))
}

func (c *ProxyCmd) handlePing(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)
}

func (c *ProxyCmd) handleProxy(cfg *config.ConfigStruct) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		// Get host from query parameter
		query := r.URL.Query()
		host := query.Get("ns")
		if host == "" {
			host = "docker.io"
		}

		slog := slog.With("raw_reg", host)

		// Use optimized routing
		srcImage, dstImage, err := c.mirror.Rewrite(r, cfg)
		if err != nil {
			slog.Error("failed to rewrite request", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Handle direct forwarding (no mirror rule)
		if srcImage == nil || dstImage == nil {
			cachedConfig := c.mirror.GetCachedConfig(host, cfg)
			if cachedConfig.Source != nil {
				// Direct forward to source registry
				forwardRequest(w, r, srcImage)
			}
			slog.Warn("no mapping rule, forwarding directly")
			return
		}

		// Check if this is a manifest request (image pull)
		routeInfo := c.mirror.ParseRoute(r.URL.Path)
		if routeInfo.Valid && routeInfo.Endpoint == "manifests" && srcImage.Tag != "" {
			// For manifest requests, we need to ensure the mirror has the latest version
			slog.Debug("manifest request detected, checking version consistency",
				"raw", srcImage.String(), "mirror", dstImage.String())

			// This will trigger copy if version mismatch or mirror image is missing
			c.versionChecker.CheckVersionConsistency(r.Context(), srcImage, dstImage)
		}

		// Forward request to mirror registry
		forwardRequest(w, r, dstImage)
	}
}

// forwardRequest forwards the request to the target image's registry
func forwardRequest(w http.ResponseWriter, r *http.Request, image *config.ImagePattern) error {
	// Build the target URL
	targetURL := fmt.Sprintf("%s://%s%s", image.Scheme, image.Registry, r.URL.Path)

	// Create a new request to the target
	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		return err
	}

	// Copy headers
	for name, values := range r.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// Set host header
	req.Host = image.Registry

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "Failed to forward request: %v", err)
		return err
	}
	defer resp.Body.Close()

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Write response status and body
	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(readResponseBody(resp))
	return err
}

func readResponseBody(resp *http.Response) []byte {
	body := make([]byte, 4096)
	var buf []byte
	for {
		n, err := resp.Body.Read(body)
		if n > 0 {
			buf = append(buf, body[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf
}
