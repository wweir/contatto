package config

import (
	"bytes"
)

// RenderMirrorPath renders the mirror path for an image using the source's path template
func (s *Source) RenderMirrorPath(project, repo, tag string) (string, error) {
	data := struct {
		Project string
		Repo    string
		Tag     string
	}{
		Project: project,
		Repo:    repo,
		Tag:     tag,
	}

	var buf bytes.Buffer
	err := s.pathTpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
