package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/wweir/contatto/config"
)

type ImageCopier struct {
	copyQueue chan CopyRequest
	config    *config.ConfigStruct
}

type CopyRequest struct {
	Src  *config.ImagePattern
	Dst  *config.ImagePattern
	Done chan struct{}
}

func NewImageCopier(cfg *config.ConfigStruct) *ImageCopier {
	c := &ImageCopier{
		copyQueue: make(chan CopyRequest, cfg.CopyQueueSize),
		config:    cfg,
	}

	// Start worker goroutines
	for i := 0; i < 10; i++ { // TODO: configurable worker count
		go c.worker()
	}

	return c
}

func (c *ImageCopier) worker() {
	for req := range c.copyQueue {
		if err := c.CopyImage(context.Background(), req.Src, req.Dst); err != nil {
			slog.Error("Failed to copy image", "src", req.Src.String(), "dst", req.Dst.String(), "error", err)
		} else {
			slog.Info("Image copied successfully", "src", req.Src.String(), "dst", req.Dst.String())
		}

		if req.Done != nil {
			close(req.Done)
		}
	}
}

func (c *ImageCopier) CopyImage(ctx context.Context, src, dst *config.ImagePattern) error {
	// Build pull options for source registry
	pullOpts := []remote.Option{remote.WithContext(ctx)}
	if srcCfg := c.config.GetSource(src.Registry); srcCfg != nil {
		transport, err := srcCfg.ProxyTransport()
		if err != nil {
			slog.Error("failed to create proxy transport", "proxy", srcCfg.Proxy, "error", err)
		} else if transport != nil {
			pullOpts = append(pullOpts, remote.WithTransport(transport))
		}
	}

	// Build push options for mirror registry
	pushOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(c.config.MirrorKeychain()),
	}

	srcRef, err := name.ParseReference(src.String())
	if err != nil {
		return fmt.Errorf("parsing src reference %q: %w", src.String(), err)
	}
	dstRef, err := name.ParseReference(dst.String())
	if err != nil {
		return fmt.Errorf("parsing dst reference %q: %w", dst.String(), err)
	}

	puller, err := remote.NewPuller(pullOpts...)
	if err != nil {
		return fmt.Errorf("create puller: %w", err)
	}
	desc, err := puller.Get(ctx, srcRef)
	if err != nil {
		return fmt.Errorf("fetching %q: %w", src.String(), err)
	}

	pusher, err := remote.NewPusher(pushOpts...)
	if err != nil {
		return fmt.Errorf("create pusher: %w", err)
	}
	return pusher.Push(ctx, dstRef, desc)
}

func (c *ImageCopier) QueueCopy(src, dst *config.ImagePattern) (chan struct{}, error) {
	done := make(chan struct{})
	select {
	case c.copyQueue <- CopyRequest{Src: src, Dst: dst, Done: done}:
		return done, nil
	default:
		return nil, ErrCopyQueueFull
	}
}

var ErrCopyQueueFull = errors.New("copy queue is full")
