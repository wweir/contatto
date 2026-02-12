package proxy

import (
	"context"
	"log/slog"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/wweir/contatto/config"
)

// VersionChecker checks version consistency between source and mirror registries.
type VersionChecker struct {
	cfg         *config.ConfigStruct
	imageCopier *ImageCopier
}

func NewVersionChecker(cfg *config.ConfigStruct, imageCopier *ImageCopier) *VersionChecker {
	return &VersionChecker{
		cfg:         cfg,
		imageCopier: imageCopier,
	}
}

// CheckVersionConsistency compares source and mirror digests, triggers copy on mismatch.
func (vc *VersionChecker) CheckVersionConsistency(ctx context.Context, srcImage, dstImage *config.ImagePattern) {
	if srcImage.Tag == "" {
		return
	}

	srcOpts := []crane.Option{crane.WithContext(ctx)}
	if src := vc.cfg.GetSource(srcImage.Registry); src != nil {
		if transport, err := src.ProxyTransport(); err != nil {
			slog.Error("failed to create proxy transport", "proxy", src.Proxy, "error", err)
		} else if transport != nil {
			srcOpts = append(srcOpts, crane.WithTransport(transport))
		}
		if src.Insecure {
			srcOpts = append(srcOpts, crane.Insecure)
		}
	}

	srcDigest, err := crane.Digest(srcImage.String(), srcOpts...)
	if err != nil {
		slog.Error("failed to get source digest", "error", err, "image", srcImage.String())
		return
	}

	dstOpts := []crane.Option{
		crane.WithContext(ctx),
		crane.WithAuthFromKeychain(vc.cfg.MirrorKeychain()),
	}
	if vc.cfg.Mirror.Insecure {
		dstOpts = append(dstOpts, crane.Insecure)
	}

	dstDigest, err := crane.Digest(dstImage.String(), dstOpts...)
	if err != nil {
		slog.Warn("failed to get mirror digest, will copy", "src", srcImage.String(), "mirror", dstImage.String(), "error", err)
		vc.triggerCopy(srcImage, dstImage, "mirror_not_found")
		return
	}

	if srcDigest != dstDigest {
		slog.Info("version mismatch, will copy",
			"src_digest", srcDigest, "dst_digest", dstDigest, "image", srcImage.String())
		vc.triggerCopy(srcImage, dstImage, "version_mismatch")
	}
}

func (vc *VersionChecker) triggerCopy(src, dst *config.ImagePattern, reason string) {
	done, err := vc.imageCopier.QueueCopy(src, dst)
	if err != nil {
		slog.Error("failed to queue image copy", "error", err, "image", src.String())
		return
	}

	slog.Info("image copy queued", "image", src.String(), "reason", reason)
	go func() {
		<-done
		slog.Info("image copy completed", "image", src.String())
	}()
}
