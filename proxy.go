package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/julienschmidt/httprouter"
	"github.com/wweir/contatto/conf"
)

type ProxyCmd struct {
	firstRequest sync.Map
}

func (c *ProxyCmd) Run(config *conf.Config) error {
	authorizer := docker.NewDockerAuthorizer(
		docker.WithAuthCreds(func(host string) (string, string, error) {
			return config.Registry[host].ReadAuthFromDockerConfig(config.DockerConfigFile)
		}))

	router := httprouter.New()
	router.GET("/v2/", httprouter.Handle(nil))
	router.HEAD("/v2/:project/:repo/manifests/:tag", httprouter.Handle(nil))
	router.GET("/v2/:project/:repo/blobs/:sha256", httprouter.Handle(nil))
	router.NotFound = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		slog.Error("rewrite missing", "method", r.Method, "url", r.URL.String())
	})

	proxy := &httputil.ReverseProxy{}
	proxy.Rewrite = func(r *httputil.ProxyRequest) {
		query := r.In.URL.Query()
		host := query.Get("ns")
		if host == "" {
			// for docker mirror, use docker.io as default registry
			host = "docker.io"
		}

		slog := slog.With("raw_reg", host)

		rule, ok := config.Rule[host]
		if !ok { // no mapping rule, directly forward to the registry
			if reg, ok := config.Registry[host]; ok {
				r.Out.URL.Host = reg.Host()
				if reg.Insecure {
					r.Out.URL.Scheme = "http"
				} else {
					r.Out.URL.Scheme = "https"
				}
			}
			slog.Warn("no mapping rule")
			return
		}

		// rewrite host, scheme, query values
		dstReg := config.Registry[rule.MirrorRegistry]
		r.Out.URL.Scheme = dstReg.Scheme()
		r.Out.Host = dstReg.Host()
		r.Out.URL.Host = r.Out.Host
		query.Set("ns", r.Out.Host)
		r.Out.URL.RawQuery = query.Encode()

		// rewrite path and tag, rendering path template
		_, ps, _ := router.Lookup(r.Out.Method, r.Out.URL.Path)
		if len(ps) == 0 {
			if r.Out.URL.Path != "/v2/" {
				slog.Error("rewrite missing", "method", r.Out.Method, "url", r.Out.URL.String(), "ps", ps)
			}
			return
		}

		srcImage := &ImagePattern{Registry: host, Alias: config.Registry[host].Alias}
		dstImage := &ImagePattern{Registry: dstReg.Host(), Alias: dstReg.Alias}

		srcImage.ParseParams(ps)
		mirrorPath, err := rule.RenderMirrorPath(srcImage)
		if err != nil {
			slog.Error("failed to render mirror path", "err", err)
			return
		}
		dstImage.ParseImage(r.Out.Host + "/" + mirrorPath)

		r.Out.URL.Path = strings.Replace(r.Out.URL.Path, srcImage.Project, dstImage.Project, 1)
		r.Out.URL.Path = strings.Replace(r.Out.URL.Path, srcImage.Repo, dstImage.Repo, 1)
		if srcImage.Tag != "" {
			r.Out.URL.Path = strings.Replace(r.Out.URL.Path, srcImage.Tag, dstImage.Tag, 1)

			r.Out.Header.Set("Contatto-Raw-Image", srcImage.String())
			r.Out.Header.Set("Contatto-Mirror-Image", dstImage.String())

			slog.Info("proxy", "mirror", dstImage)
		}

		// add auth header
		if _, ok := c.firstRequest.LoadOrStore(dstImage.String(), struct{}{}); !ok {
			u := *r.Out.URL
			u.Path, u.RawQuery = "/v2/", ""
			resp, err := http.Get(u.String())
			if err != nil {
				slog.Error("failed to get", "err", err)
			} else {
				defer resp.Body.Close()
				if resp.StatusCode == 401 {
					authorizer.AddResponses(r.Out.Context(), []*http.Response{resp})
				}
			}
		}

		ctx := docker.ContextWithAppendPullRepositoryScope(r.Out.Context(), dstImage.Project+"/"+dstImage.Repo)
		if err := authorizer.Authorize(ctx, r.Out); err != nil {
			slog.Error("failed to authorize", "err", err)
			return
		}
	}
	proxy.ModifyResponse = func(w *http.Response) error {
		switch w.StatusCode {
		case 200, 307:
		case 401:
			slog.Debug("auth failed", "url", w.Request.URL.String())
			if err := authorizer.AddResponses(w.Request.Context(), []*http.Response{w}); err != nil {
				slog.Error("failed to add responses", "err", err)
			}

			c.RetryToRewriteResp(w, "auth", func(req *http.Request) (*http.Response, error) {
				if err := authorizer.Authorize(req.Context(), req); err != nil {
					return nil, fmt.Errorf("failed to authorize: %w", err)
				}
				return http.DefaultClient.Do(req)
			})

		case 404:
			rawStr := w.Request.Header.Get("Contatto-Raw-Image")
			mirrorStr := w.Request.Header.Get("Contatto-Mirror-Image")
			if rawStr == "" || mirrorStr == "" {
				slog.Debug("missing image header", "url", w.Request.URL.String())
				return nil
			}

			raw := (&ImagePattern{}).ParseImage(rawStr)
			raw.Alias = config.Registry[raw.Registry].Alias
			mirror := (&ImagePattern{}).ParseImage(mirrorStr)
			mirror.Alias = config.Registry[mirror.Registry].Alias

			slog := slog.With("raw_reg", raw.Registry)
			rule := config.Rule[raw.Registry]
			cmdline, err := rule.RenderOnMissingCmd(map[string]any{
				"Raw": raw, "Mirror": mirror, "raw": raw.String(), "mirror": mirror.String(),
			})
			if err != nil {
				slog.Error("failed to render on missing command", "err", err)
				return nil
			}

			if cmdline != "" {
				slog.Info("mirror image not exist, run on missing command", "cmd", cmdline)
				startTime := time.Now()
				cmd := exec.Command("sh", "-c", cmdline)
				out, err := cmd.CombinedOutput()
				if err != nil {
					slog.Error("failed to run on missing command", "output", string(out), "err", err)
					return nil
				}
				slog.Info("on missing command finished", "took", time.Since(startTime))

				c.RetryToRewriteResp(w, "on_missing", http.DefaultClient.Do)
			}

		default:
			slog.Info(w.Request.Method, "url", w.Request.URL.String(), "status", w.StatusCode)
		}
		return nil
	}

	return http.ListenAndServe(config.Addr, proxy)
}

func (c *ProxyCmd) RetryToRewriteResp(w *http.Response, reason string, do func(req *http.Request) (*http.Response, error)) {
	req := w.Request.Clone(w.Request.Context())
	req.RequestURI = ""
	resp, err := do(req)
	if err != nil {
		slog.Warn("failed to retry request", "reason", reason, "err", err)
		return
	}

	w.StatusCode = resp.StatusCode
	w.Status = resp.Status
	w.Body = resp.Body

	slog.Info("rewrite response", "reason", reason, "url", req.URL.String())
}

type ImagePattern struct {
	Registry string
	Alias    string
	Project  string
	Repo     string
	Tag      string
}

// docker.io/library/alpine:latest
func (p *ImagePattern) ParseImage(image string) *ImagePattern {
	sepSlashFirst := strings.IndexByte(image, '/')
	p.Registry = image[:sepSlashFirst]
	sepSlashLast := strings.LastIndexByte(image, '/')
	p.Project = image[sepSlashFirst+1 : sepSlashLast]
	sepColonIdx := strings.LastIndexByte(image, ':')
	p.Repo = image[sepSlashLast+1 : sepColonIdx]
	p.Tag = image[sepColonIdx+1:]
	return p
}

func (p *ImagePattern) ParseParams(params httprouter.Params) *ImagePattern {
	for _, param := range params {
		switch param.Key {
		case "project":
			p.Project = param.Value
		case "repo":
			p.Repo = param.Value
		case "tag":
			p.Tag = param.Value
		}
	}
	return p
}

func (p *ImagePattern) String() string {
	return p.Registry + "/" + p.Project + "/" + p.Repo + ":" + p.Tag
}
