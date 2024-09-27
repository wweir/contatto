package main

import (
	"context"
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
	"github.com/wweir/contatto/etc"
)

type ProxyCmd struct {
	Config string `short:"c" required:"" default:"/etc/contatto/config.toml"`

	firstAttach sync.Map
}

func (c *ProxyCmd) Run() error {
	if err := etc.InitConfig(c.Config); err != nil {
		return err
	}

	authorizer := docker.NewDockerAuthorizer(docker.WithAuthCreds(c.AuthCreds),
		docker.WithFetchRefreshToken(func(ctx context.Context, refreshToken string, req *http.Request) {
			slog.Info("fetch refresh token", "refreshToken", refreshToken, "url", req.URL.String())
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
			host = "docker.io"
		}

		rule, ok := etc.RuleHostM[host]
		if !ok { // no mapping rule, directly forward to the registry
			if reg, ok := etc.RegHostM[host]; ok {
				r.Out.URL.Host = reg.Host
				if reg.Insecure {
					r.Out.URL.Scheme = "http"
				} else {
					r.Out.URL.Scheme = "https"
				}
			}
			slog.Warn("no mapping rule", "url", r.Out.URL.String())
			return
		}

		srcPat := &ImagePattern{Registry: host}
		dstPat := &ImagePattern{}

		{ // rewrite host, scheme, query
			dstReg := etc.RegM[etc.Config.MirrorRegistry]
			dstPat.Registry = dstReg.Host

			if dstReg.Insecure {
				r.Out.URL.Scheme = "http"
			} else {
				r.Out.URL.Scheme = "https"
			}
			r.Out.Host = dstReg.Host
			r.Out.URL.Host = dstReg.Host
			query.Set("ns", dstReg.Host)
			r.Out.URL.RawQuery = query.Encode()
		}

		{ // rewrite path, follow the mapping rule
			_, ps, _ := router.Lookup(r.Out.Method, r.Out.URL.Path)
			if len(ps) == 0 {
				slog.Error("rewrite missing", "method", r.Out.Method, "url", r.Out.URL.String(), "ps", ps)
			}

			srcPat.ParseParams(ps)
			mirrorPath, err := rule.RenderMirrorPath(srcPat)
			if err != nil {
				slog.Error("failed to render mirror path", "err", err)
				return
			}

			dstPat.ParseImage(r.Out.Host + "/" + mirrorPath)
			r.Out.URL.Path = strings.Replace(r.Out.URL.Path, srcPat.Project, dstPat.Project, 1)
			r.Out.URL.Path = strings.Replace(r.Out.URL.Path, srcPat.Repo, dstPat.Repo, 1)
			r.Out.URL.Path = strings.Replace(r.Out.URL.Path, srcPat.Tag, dstPat.Tag, 1)

			r.Out.Header.Set("Contatto-Raw-Image", srcPat.String())
			r.Out.Header.Set("Contatto-Mirror-Image", dstPat.String())
			slog.Info("proxy", "raw", srcPat, "mirror", dstPat)
		}

		// the first time to access the registry, do a HEAD request to get the auth method
		if _, ok := c.firstAttach.LoadOrStore(r.Out.Host, struct{}{}); !ok {
			resp, err := http.Head(r.Out.URL.String())
			if err != nil {
				slog.Warn("failed to head", "url", r.Out.URL.String(), "err", err)
			} else if resp.StatusCode == 401 {
				if err := authorizer.AddResponses(context.Background(), []*http.Response{resp}); err != nil {
					slog.Error("failed to add responses", "err", err)
				}
			}
		}

		// add auth header
		if err := authorizer.Authorize(context.Background(), r.Out); err != nil {
			slog.Error("failed to authorize", "err", err)
			return
		}
	}
	proxy.ModifyResponse = func(w *http.Response) error {
		switch w.StatusCode {
		case 401:
			if err := authorizer.AddResponses(context.Background(), []*http.Response{w}); err != nil {
				slog.Error("failed to add responses", "err", err)
			}
		case 404:
			var raw, mirror ImagePattern
			raw.ParseImage(w.Request.Header.Get("Contatto-Raw-Image"))
			mirror.ParseImage(w.Request.Header.Get("Contatto-Mirror-Image"))
			if etc.OnMissing != nil {
				cmdline, err := etc.OnMissing(map[string]any{
					"Raw": raw, "Mirror": mirror, "raw": raw.String(), "mirror": mirror.String(),
				})
				if err != nil {
					slog.Error("failed to render on missing command", "err", err)
					return err
				}
				slog.Info("mirror image not exist, run on missing command", "cmd", cmdline)

				go func() {
					startTime := time.Now()
					cmd := exec.Command("sh", "-c", cmdline)
					out, err := cmd.CombinedOutput()
					if err != nil {
						slog.Error("failed to run on missing command", "output", out, "err", err)
						return
					}
					slog.Info("on missing command finished", "cost", time.Since(startTime))
				}()
			}

			// cmd := exec.Command("sh", "-c", "")
			// if err := cmd.Run(); err != nil {
			// 	slog.Error("failed to pull image", "err", err)
			// }
		default:
			slog.Info(w.Request.Method, "url", w.Request.URL.String(), "status", w.StatusCode)
		}
		return nil
	}

	return http.ListenAndServe(etc.Config.Addr, proxy)
}

func (c *ProxyCmd) AuthCreds(host string) (string, string, error) {
	slog.Info("read auth creds", "registry", host)

	reg := etc.RegHostM[host]
	if reg.UserName != "" && reg.Password != "" {
		return reg.UserName, reg.Password, nil
	}

	if auth, ok := etc.DockerAuth[host]; ok {
		return auth.UserName, auth.Password, nil
	}

	return "", "", fmt.Errorf(
		"registry (%s) user/password not set and docker config file not set", host)
}

type ImagePattern struct {
	Registry string
	Project  string
	Repo     string
	Tag      string
}

// docker.io/library/alpine:latest
func (p *ImagePattern) ParseImage(image string) {
	sepSlashFirst := strings.IndexByte(image, '/')
	p.Registry = image[:sepSlashFirst]
	sepSlashLast := strings.LastIndexByte(image, '/')
	p.Project = image[sepSlashFirst+1 : sepSlashLast]
	sepColonIdx := strings.LastIndexByte(image, ':')
	p.Repo = image[sepSlashLast+1 : sepColonIdx]
	p.Tag = image[sepColonIdx+1:]
}

func (p *ImagePattern) ParseParams(params httprouter.Params) {
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
}

func (p *ImagePattern) String() string {
	return p.Registry + "/" + p.Project + "/" + p.Repo + ":" + p.Tag
}
