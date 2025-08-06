package app

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

	*w = *resp
	return nil
}

func SafeRewriteFile(file string, writeFn func(w io.Writer) error) error {
	tempFile := file + "_contatto_tmp"
	f, err := os.OpenFile(tempFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create temp file(%s): %w", tempFile, err)
	}

	if err := writeFn(f); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("write temp file: %w", err)
	}
	f.Close()

	backupFile := file + "_contatto_backup"
	if _, err := os.Stat(file); err == nil {
		if _, err := os.Stat(backupFile); os.IsNotExist(err) {
			if err := os.Rename(file, backupFile); err != nil {
				os.Remove(tempFile)
				return fmt.Errorf("create backup of %s: %w", file, err)
			}
		} else {
			if err := os.Remove(file); err != nil {
				os.Remove(tempFile)
				return fmt.Errorf("remove original file %s: %w", file, err)
			}
		}
	}

	if err := os.Rename(tempFile, file); err != nil {
		if _, err := os.Stat(backupFile); err == nil {
			os.Rename(backupFile, file)
		}
		return fmt.Errorf("move temp file to %s: %w", file, err)
	}

	return nil
}
