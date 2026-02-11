package app

import (
	"fmt"
	"net/http"
)

func RetryToRewriteResp(w *http.Response, reason string,
	do func(req *http.Request) (*http.Response, error),
) error {
	req := w.Request.Clone(w.Request.Context())
	req.RequestURI = ""
	resp, err := do(req)
	if err != nil {
		return fmt.Errorf("failed to retry request: %w", err)
	}

	*w = *resp
	return nil
}
