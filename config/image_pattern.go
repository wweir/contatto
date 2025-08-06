package config

import (
	"errors"
	"strings"

	"github.com/julienschmidt/httprouter"
)

var (
	ErrInvalidImageFormat = errors.New("invalid image format")
	ErrEmptyImageString   = errors.New("empty image string")
)

// ImagePattern represents a container image reference with its components
type ImagePattern struct {
	Scheme   string
	Registry string
	Alias    string
	Project  string
	Repo     string
	Tag      string
}

// ParseImage parses an image string in format: registry/project/repo:tag
// Examples: docker.io/library/alpine:latest, quay.io/prometheus/prometheus:v2.40.0
func (p *ImagePattern) ParseImage(image string) error {
	if image == "" {
		return ErrEmptyImageString
	}

	// Find first slash (separates registry from project)
	sepSlashFirst := strings.IndexByte(image, '/')
	if sepSlashFirst == -1 {
		return ErrInvalidImageFormat
	}

	// Find last slash (separates project from repo)
	sepSlashLast := strings.LastIndexByte(image, '/')
	if sepSlashLast == -1 || sepSlashLast <= sepSlashFirst {
		return ErrInvalidImageFormat
	}

	// Find colon (separates repo from tag)
	sepColonIdx := strings.LastIndexByte(image, ':')
	if sepColonIdx == -1 || sepColonIdx <= sepSlashLast {
		return ErrInvalidImageFormat
	}

	p.Registry = image[:sepSlashFirst]
	p.Project = image[sepSlashFirst+1 : sepSlashLast]
	p.Repo = image[sepSlashLast+1 : sepColonIdx]
	p.Tag = image[sepColonIdx+1:]

	// Validate components are not empty
	if p.Registry == "" || p.Project == "" || p.Repo == "" || p.Tag == "" {
		return ErrInvalidImageFormat
	}

	return nil
}

// ParseParams extracts route parameters from httprouter.Params
func (p *ImagePattern) ParseParams(params httprouter.Params) {
	for _, param := range params {
		switch param.Key {
		case "project":
			p.Project = param.Value
		case "repo":
			p.Repo = param.Value
		case "tag":
			p.Tag = param.Value
		case "digest":
			p.Tag = param.Value
		}
	}
}

// String returns the full image reference
func (p *ImagePattern) String() string {
	return p.Registry + "/" + p.Project + "/" + p.Repo + ":" + p.Tag
}

// BuildPath constructs a Docker registry API path for the given endpoint
func (p *ImagePattern) BuildPath(endpoint string) string {
	switch endpoint {
	case "manifests":
		return "/v2/" + p.Project + "/" + p.Repo + "/manifests/" + p.Tag
	case "blobs":
		return "/v2/" + p.Project + "/" + p.Repo + "/blobs/" + p.Tag
	default:
		return "/v2/" + p.Project + "/" + p.Repo + "/" + endpoint + "/" + p.Tag
	}
}

// Validate checks if all required fields are present
func (p *ImagePattern) Validate() error {
	if p.Registry == "" {
		return errors.New("registry is required")
	}
	if p.Project == "" {
		return errors.New("project is required")
	}
	if p.Repo == "" {
		return errors.New("repo is required")
	}
	if p.Tag == "" {
		return errors.New("tag is required")
	}
	return nil
}
