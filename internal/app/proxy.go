package app

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/wweir/contatto/config"
	"github.com/wweir/contatto/internal/proxy"
)

type ProxyCmd struct {
	mirror         *proxy.Mirror
	versionChecker *proxy.VersionChecker
	imageCopier    *proxy.ImageCopier
}

func (c *ProxyCmd) Run(cfg *config.ConfigStruct) error {
	slog.Info("Starting proxy", "version", config.Version, "date", config.Date, "config", cfg)

	c.mirror = proxy.NewMirror(cfg)
	c.imageCopier = proxy.NewImageCopier(cfg)
	c.versionChecker = proxy.NewVersionChecker(cfg, c.imageCopier)

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

	router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.URL.Query().Get("ns")
		if host == "" {
			host = "docker.io"
		}
		slog.Warn("unknown route, forwarding directly", "method", r.Method, "path", r.URL.Path, "host", host)

		scheme := "https"
		if src := cfg.GetSource(host); src != nil && src.Insecure {
			scheme = "http"
		}
		forwardRequest(w, r, &config.ImagePattern{Scheme: scheme, Registry: host}, nil)
	})
}

func (c *ProxyCmd) handlePing(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)
}

func (c *ProxyCmd) handleProxy(cfg *config.ConfigStruct) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		host := r.URL.Query().Get("ns")
		if host == "" {
			host = "docker.io"
		}

		project := params.ByName("project")
		repo := params.ByName("repo")
		tag := params.ByName("reference")
		if tag == "" {
			tag = params.ByName("digest")
		}

		slog := slog.With("host", host)

		srcImage, dstImage, err := c.mirror.Rewrite(r, project, repo, tag)
		if err != nil {
			slog.Error("failed to rewrite request", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// No mapping rule, forward directly to source
		if srcImage == nil || dstImage == nil {
			src := cfg.GetSource(host)
			scheme := "https"
			if src != nil && src.Insecure {
				scheme = "http"
			}
			slog.Warn("no mapping rule, forwarding directly")
			forwardRequest(w, r, &config.ImagePattern{
				Scheme: scheme, Registry: host,
				Project: project, Repo: repo, Tag: tag,
			}, sourceHTTPClient(src))
			return
		}

		// For manifest requests, check version consistency
		if tag != "" && params.ByName("reference") != "" {
			slog.Debug("checking version consistency", "src", srcImage.String(), "mirror", dstImage.String())
			c.versionChecker.CheckVersionConsistency(r.Context(), srcImage, dstImage)
		}

		forwardRequest(w, r, dstImage, nil)
	}
}

func sourceHTTPClient(src *config.Source) *http.Client {
	if src == nil {
		return nil
	}
	transport, err := src.ProxyTransport()
	if err != nil {
		slog.Error("failed to create proxy transport", "proxy", src.Proxy, "error", err)
		return nil
	}
	if transport == nil {
		return nil
	}
	return &http.Client{Transport: transport}
}

func forwardRequest(w http.ResponseWriter, r *http.Request, image *config.ImagePattern, client *http.Client) {
	if client == nil {
		client = http.DefaultClient
	}

	registry := image.Registry
	if registry == "docker.io" {
		registry = "registry-1.docker.io"
	}

	targetURL := fmt.Sprintf("%s://%s%s", image.Scheme, registry, r.URL.Path)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.Header = r.Header.Clone()
	req.Host = registry

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to forward request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
