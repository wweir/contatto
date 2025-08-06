package config

import (
	"testing"

	"github.com/julienschmidt/httprouter"
)

func TestImagePattern_ParseImage(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    ImagePattern
		expectError bool
	}{
		{
			name:  "valid docker.io image",
			input: "docker.io/library/alpine:latest",
			expected: ImagePattern{
				Registry: "docker.io",
				Project:  "library",
				Repo:     "alpine",
				Tag:      "latest",
			},
			expectError: false,
		},
		{
			name:  "valid custom registry",
			input: "quay.io/prometheus/prometheus:v2.40.0",
			expected: ImagePattern{
				Registry: "quay.io",
				Project:  "prometheus",
				Repo:     "prometheus",
				Tag:      "v2.40.0",
			},
			expectError: false,
		},
		{
			name:        "empty image string",
			input:       "",
			expectError: true,
		},
		{
			name:        "invalid format - no slash",
			input:       "alpine:latest",
			expectError: true,
		},
		{
			name:        "invalid format - no colon",
			input:       "docker.io/library/alpine",
			expectError: true,
		},
		{
			name:        "invalid format - malformed",
			input:       "docker.io:latest",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ImagePattern{}
			err := p.ParseImage(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if p.Registry != tt.expected.Registry {
				t.Errorf("Registry = %v, want %v", p.Registry, tt.expected.Registry)
			}
			if p.Project != tt.expected.Project {
				t.Errorf("Project = %v, want %v", p.Project, tt.expected.Project)
			}
			if p.Repo != tt.expected.Repo {
				t.Errorf("Repo = %v, want %v", p.Repo, tt.expected.Repo)
			}
			if p.Tag != tt.expected.Tag {
				t.Errorf("Tag = %v, want %v", p.Tag, tt.expected.Tag)
			}
		})
	}
}

func TestImagePattern_BuildPath(t *testing.T) {
	p := &ImagePattern{
		Project: "library",
		Repo:    "alpine",
		Tag:     "latest",
	}

	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "manifests endpoint",
			endpoint: "manifests",
			expected: "/v2/library/alpine/manifests/latest",
		},
		{
			name:     "blobs endpoint",
			endpoint: "blobs",
			expected: "/v2/library/alpine/blobs/latest",
		},
		{
			name:     "custom endpoint",
			endpoint: "tags",
			expected: "/v2/library/alpine/tags/latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.BuildPath(tt.endpoint)
			if result != tt.expected {
				t.Errorf("BuildPath(%s) = %v, want %v", tt.endpoint, result, tt.expected)
			}
		})
	}
}

func TestImagePattern_ParseParams(t *testing.T) {
	params := httprouter.Params{
		{Key: "project", Value: "library"},
		{Key: "repo", Value: "alpine"},
		{Key: "tag", Value: "latest"},
	}

	p := &ImagePattern{}
	p.ParseParams(params)

	if p.Project != "library" {
		t.Errorf("Project = %v, want library", p.Project)
	}
	if p.Repo != "alpine" {
		t.Errorf("Repo = %v, want alpine", p.Repo)
	}
	if p.Tag != "latest" {
		t.Errorf("Tag = %v, want latest", p.Tag)
	}
}

// Benchmark tests
func BenchmarkImagePattern_ParseImage(b *testing.B) {
	p := &ImagePattern{}
	image := "docker.io/library/alpine:latest"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.ParseImage(image)
	}
}

func BenchmarkImagePattern_BuildPath(b *testing.B) {
	p := &ImagePattern{
		Project: "library",
		Repo:    "alpine",
		Tag:     "latest",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.BuildPath("manifests")
	}
}

func BenchmarkImagePattern_String(b *testing.B) {
	p := &ImagePattern{
		Registry: "docker.io",
		Project:  "library",
		Repo:     "alpine",
		Tag:      "latest",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.String()
	}
}
