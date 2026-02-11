package proxy

import (
	"context"
	"log/slog"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/transports/alltransports"
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
	srcRef, err := alltransports.ParseImageName("docker://" + src.String())
	if err != nil {
		return err
	}

	dstRef, err := alltransports.ParseImageName("docker://" + dst.String())
	if err != nil {
		return err
	}

	_, err = copy.Image(ctx, nil, dstRef, srcRef, &copy.Options{})
	return err
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

var ErrCopyQueueFull = config.ErrInvalidImageFormat // TODO: create specific error
