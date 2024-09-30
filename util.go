package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
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

	w.StatusCode = resp.StatusCode
	w.Status = resp.Status
	w.Body = resp.Body

	return nil
}

func SafeRewriteFile(file string, writeFn func(w io.Writer) error) error {
	f, err := os.OpenFile(file+"_safe", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file(%s): %w", file, err)
	}

	if err := writeFn(f); err != nil {
		f.Close()
		return err
	}
	f.Close()

	backupFile := file + "_bak"
	isBackuped := false
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		isBackuped = os.Rename(file, backupFile) == nil
	}

	if err := os.Rename(file+"_safe", file); err != nil {
		if isBackuped {
			os.Rename(backupFile, file)
		}
		return fmt.Errorf("mv file(%s): %w", file, err)
	}

	return nil
}
