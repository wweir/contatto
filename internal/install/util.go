package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// nestedMapEnsure ensures all nested keys exist in a map chain, returning the deepest map.
func nestedMapEnsure(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if m[key] == nil {
			m[key] = map[string]any{}
		}
		m = m[key].(map[string]any)
	}
	return m
}

func mapStr(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func hasNonEmptyMap(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		if sub, ok := m[key].(map[string]any); ok && len(sub) != 0 {
			return true
		}
	}
	return false
}

// ConfirmFunc asks user a yes/no question, returns true if confirmed.
type ConfirmFunc func(label string) bool

// PromptFunc asks user to input or confirm a value. Returns the final value.
type PromptFunc func(label, defaultValue string) string

func createConfigFileIfNotExists(path, content string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("create config file %s: %w", path, err)
	}
	fmt.Printf("  ✓ Created config file: %s\n", path)
	return nil
}

func safeRewriteFile(file string, writeFn func(w io.Writer) error) error {
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

	// backup original file (only first time)
	backupFile := file + "_contatto_backup"
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		os.Rename(file, backupFile)
	} else {
		os.Remove(file)
	}

	if err := os.Rename(tempFile, file); err != nil {
		// rollback
		os.Rename(backupFile, file)
		return fmt.Errorf("move temp file to %s: %w", file, err)
	}
	return nil
}
