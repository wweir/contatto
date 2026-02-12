package proxy

import (
	"net/http"

	"github.com/wweir/contatto/config"
)

// Mirror handles request rewriting from source registries to mirror registry.
type Mirror struct {
	cfg *config.ConfigStruct
}

func NewMirror(cfg *config.ConfigStruct) *Mirror {
	return &Mirror{cfg: cfg}
}

// Rewrite maps a source registry request to source and mirror image patterns.
// Returns (nil, nil, nil) if no mapping rule exists for the host.
func (m *Mirror) Rewrite(r *http.Request, project, repo, tag string) (*config.ImagePattern, *config.ImagePattern, error) {
	host := r.URL.Query().Get("ns")
	if host == "" {
		host = "docker.io"
	}

	src := m.cfg.GetSource(host)
	if src == nil {
		return nil, nil, nil
	}

	srcImage := &config.ImagePattern{
		Registry: host,
		Project:  project,
		Repo:     repo,
		Tag:      tag,
	}
	srcImage.SetInsecure(src.Insecure)

	if err := srcImage.Validate(); err != nil {
		return nil, nil, err
	}

	mirrorPath, err := src.RenderMirrorPath(project, repo, tag)
	if err != nil {
		return nil, nil, err
	}

	dstImage := &config.ImagePattern{Registry: m.cfg.Mirror.Registry}
	if err := dstImage.ParseImage(m.cfg.Mirror.Registry + "/" + mirrorPath); err != nil {
		return nil, nil, err
	}
	dstImage.SetInsecure(m.cfg.Mirror.Insecure)

	return srcImage, dstImage, nil
}
