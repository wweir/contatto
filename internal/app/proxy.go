package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/sower-proxy/deferlog/v2"
	"github.com/wweir/contatto/config"
	"github.com/wweir/contatto/internal/proxy"
)

type ProxyCmd struct {
	firstRequest    sync.Map
	mirror          *proxy.Mirror
	versionChecker  *proxy.VersionChecker
	commandExecutor *proxy.CommandExecutor
}

func (c *ProxyCmd) Run(cfg *config.ConfigStruct) error {
	slog.Info("Starting proxy", "version", config.Version, "date", config.Date, "config", cfg)

	// Initialize mirror
	c.mirror = proxy.NewMirror()

	// Initialize unified command executor
	queueSize := 100
	if cfg.VersionCheck != nil && cfg.VersionCheck.MaxQueueSize > 0 {
		queueSize = cfg.VersionCheck.MaxQueueSize
	}
	c.commandExecutor = proxy.NewCommandExecutor(cfg, queueSize)

	// Initialize version checker with command executor
	c.versionChecker = proxy.NewVersionChecker(c.mirror, cfg, c.commandExecutor)

	// Prewarm cache with common registries
	c.mirror.PrewarmCache(cfg)

	authorizer := docker.NewDockerAuthorizer(
		docker.WithAuthCreds(func(host string) (string, string, error) {
			if reg, ok := cfg.Registry[host]; ok {
				return reg.ReadAuthFromDockerConfig(cfg.DockerConfigFile)
			}
			return "", "", nil
		}))

	proxy := &httputil.ReverseProxy{}
	proxy.Rewrite = c.rewrite(cfg, authorizer)
	proxy.ModifyResponse = c.optimizedModifyResponse(cfg, authorizer)

	return http.ListenAndServe(cfg.Addr, proxy)
}

// rewrite provides optimized request rewriting using the route optimizer
func (c *ProxyCmd) rewrite(cfg *config.ConfigStruct, authorizer docker.Authorizer) func(r *httputil.ProxyRequest) {
	return func(r *httputil.ProxyRequest) {
		// Get host from query parameter
		query := r.In.URL.Query()
		host := query.Get("ns")
		if host == "" {
			host = "docker.io"
		}

		slog := slog.With("raw_reg", host)

		// Use optimized routing
		srcImage, dstImage, err := c.mirror.Rewrite(r.In, cfg)
		if err != nil {
			slog.Error("failed to rewrite request", "err", err)
			return
		}

		// Handle direct forwarding (no mirror rule)
		if srcImage == nil || dstImage == nil {
			cachedConfig := c.mirror.GetCachedConfig(host, cfg)
			if cachedConfig.Registry != nil {
				r.Out.URL.Host = cachedConfig.Registry.Host()
				if cachedConfig.Registry.Insecure {
					r.Out.URL.Scheme = "http"
				} else {
					r.Out.URL.Scheme = "https"
				}
			}
			slog.Warn("no mapping rule, forwarding directly")
			return
		}

		// Update request with destination information
		r.Out.URL.Scheme = dstImage.Scheme
		r.Out.URL.Host = dstImage.Registry
		r.Out.Host = dstImage.Registry

		// Update query parameters
		query.Set("ns", dstImage.Registry)
		r.Out.URL.RawQuery = query.Encode()

		// Build optimized path using the new BuildPath method
		routeInfo := c.mirror.ParseRoute(r.In.URL.Path)
		if routeInfo.Valid && routeInfo.Endpoint != "ping" {
			r.Out.URL.Path = dstImage.BuildPath(routeInfo.Endpoint)
		}

		// Set headers for debugging and fallback
		if srcImage.Tag != "" {
			r.Out.Header.Set("Contatto-Raw-Image", srcImage.String())
			r.Out.Header.Set("Contatto-Mirror-Image", dstImage.String())
			slog.Info("proxy", "mirror", dstImage.String())
		}

		// Trigger version consistency check for latest tags
		c.versionChecker.CheckVersionConsistency(r.In.Context(), srcImage, dstImage)

		// Optimized authorization with async first request handling
		c.handleAuthorizationAsync(r.Out, dstImage, authorizer)
	}
}

// handleAuthorizationAsync handles authorization asynchronously for better performance
func (c *ProxyCmd) handleAuthorizationAsync(req *http.Request, dstImage *config.ImagePattern, authorizer docker.Authorizer) {
	// Check if this is the first request for this image (for auth discovery)
	if _, loaded := c.firstRequest.LoadOrStore(dstImage.String(), struct{}{}); !loaded {
		// This is the first request, we might need to discover auth requirements
		// Do this asynchronously to not block the main request
		go func() {
			u := *req.URL
			u.Path, u.RawQuery = "/v2/", ""
			resp, err := c.mirror.GetHTTPClient().Get(u.String())
			if err != nil {
				slog.Error("failed to discover auth requirements", "err", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == 401 {
				if err := authorizer.AddResponses(req.Context(), []*http.Response{resp}); err != nil {
					slog.Error("failed to add auth responses", "err", err)
				}
			}
		}()
	}

	// Always try to authorize the request
	ctx := docker.ContextWithAppendPullRepositoryScope(req.Context(), dstImage.Project+"/"+dstImage.Repo)
	if err := authorizer.Authorize(ctx, req); err != nil {
		slog.Error("failed to authorize", "err", err)
	}
}

// optimizedModifyResponse provides optimized response modification
func (c *ProxyCmd) optimizedModifyResponse(cfg *config.ConfigStruct, authorizer docker.Authorizer) func(w *http.Response) error {
	return func(w *http.Response) (err error) {
		defer func() { deferlog.DebugWarn(err, "ModifyResponse"); err = nil }()

		switch w.StatusCode {
		case 200, 307:
			// Success cases, no action needed
			return nil

		case 401:
			slog.Debug("auth failed", "url", w.Request.URL.String())
			if err := authorizer.AddResponses(w.Request.Context(), []*http.Response{w}); err != nil {
				return fmt.Errorf("failed to add responses: %w", err)
			}

			return RetryToRewriteResp(w, "auth", func(req *http.Request) (*http.Response, error) {
				if err := authorizer.Authorize(req.Context(), req); err != nil {
					return nil, fmt.Errorf("failed to authorize: %w", err)
				}
				return c.mirror.GetHTTPClient().Do(req)
			})

		case 404:
			rawStr := w.Request.Header.Get("Contatto-Raw-Image")
			mirrorStr := w.Request.Header.Get("Contatto-Mirror-Image")
			if rawStr == "" || mirrorStr == "" {
				slog.Debug("missing image header", "url", w.Request.URL.String())
				return nil
			}

			raw := &config.ImagePattern{}
			if err := raw.ParseImage(rawStr); err != nil {
				slog.Error("failed to parse raw image", "err", err, "image", rawStr)
				return nil
			}
			raw.Alias = cfg.Registry[raw.Registry].Alias

			mirror := &config.ImagePattern{}
			if err := mirror.ParseImage(mirrorStr); err != nil {
				slog.Error("failed to parse mirror image", "err", err, "image", mirrorStr)
				return nil
			}
			mirror.Alias = cfg.Registry[mirror.Registry].Alias

			slog := slog.With("raw_reg", raw.Registry)
			rule := cfg.Rule[raw.Registry]
			cmdline, err := rule.RenderOnMissingCmd(map[string]any{
				"Raw": raw, "Mirror": mirror, "raw": raw.String(), "mirror": mirror.String(),
			})
			if err != nil {
				return fmt.Errorf("failed to render on missing command: %w", err)
			}

			if cmdline != "" {
				slog.Info("mirror image not exist, queueing on missing command", "cmd", cmdline)

				// Use unified command executor
				cmdReq := proxy.CommandRequest{
					Command:     cmdline,
					RawImage:    raw,
					MirrorImage: mirror,
					Reason:      "404_not_found",
					Source:      "404_handler",
				}

				c.commandExecutor.ExecuteCommand(cmdReq)

				if err := RetryToRewriteResp(w, "on_missing", c.mirror.GetHTTPClient().Do); err != nil {
					return fmt.Errorf("failed to rewrite response: %w", err)
				}
			}

		default:
			slog.Info(w.Request.Method, "url", w.Request.URL.String(), "status", w.StatusCode)
		}
		return nil
	}
}
