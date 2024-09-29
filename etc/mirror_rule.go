package etc

import (
	"bytes"
	"sync"
	"text/template"
)

type MirrorRule struct {
	MirrorRegistry string
	PathTpl        string // rendering a image path: /docker-hub/alpine:latest
	pathTpl        *template.Template
	OnMissingTpl   string
	onMissingTpl   *template.Template
}

func (r *MirrorRule) ParseTemplate() (err error) {
	if r.PathTpl != "" {
		r.pathTpl, err = template.New("path").Parse(r.PathTpl)
		if err != nil {
			return err
		}
	}

	if r.OnMissingTpl != "" {
		r.onMissingTpl, err = template.New("on_missing").Parse(r.OnMissingTpl)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *MirrorRule) RenderMirrorPath(param any) (string, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		bufferPool.Put(buf)
	}()

	if err := r.pathTpl.Execute(buf, param); err != nil {
		return "", err
	}

	return buf.String(), nil
}

var bufferPool = sync.Pool{
	New: func() any { return bytes.NewBuffer(nil) },
}

func (r *MirrorRule) RenderOnMissingCmd(param any) (string, error) {
	if r == nil || r.onMissingTpl == nil {
		return "", nil
	}

	buf := bufferPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		bufferPool.Put(buf)
	}()

	if err := r.onMissingTpl.Execute(buf, param); err != nil {
		return "", err
	}

	return buf.String(), nil
}
