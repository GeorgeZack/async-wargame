package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The config file holds the same settings as the command-line flags, one
// per line, so a server starts with just ./async-wargame:
//
//	# async-wargame.conf
//	addr = 127.0.0.1:8080
//	data = data/campaigns.json
//	ai = opencode
//	ai-key-file = ~/.config/opencode/api-key.env
//
// Flags on the command line win over the file. A few settings are secrets
// and only go in the file (or the environment), never on the command line
// where the process list shows them: ai-key, ai-key-file, image-key, image-key-file, host-password,
// and push-contact.

const defaultConfig = "async-wargame.conf"

// secrets are config-only settings, put in the environment for the rest of
// the program to read. A variable already set in the environment wins.
var secrets = map[string]string{
	"ai-key":        "AW_AI_KEY",
	"image-key":     "AW_IMAGE_KEY",
	"host-password": "AW_HOST_PASSWORD",
	"push-contact":  "AW_PUSH_CONTACT",
}

// loadConfig applies the config file named by -config (or
// async-wargame.conf in the working directory, if there is one) to the
// flag set before the command line is parsed. It returns the file it used.
func loadConfig(fs *flag.FlagSet, args []string) (string, error) {
	path, explicit := configPath(args)
	if path == "" {
		return "", nil
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) && !explicit {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return "", fmt.Errorf("%s:%d: expected name = value", path, n)
		}
		key, value = strings.TrimSpace(key), unquote(strings.TrimSpace(value))
		switch {
		case key == "config":
			return "", fmt.Errorf("%s:%d: a config file can't name another one", path, n)
		case key == "ai-key-file" || key == "image-key-file":
			env, names := "AW_AI_KEY", []string{"AW_AI_KEY", "OPENCODE_API_KEY"}
			if key == "image-key-file" {
				env, names = "AW_IMAGE_KEY", []string{"AW_IMAGE_KEY"}
			}
			k, err := readNamedKeyFile(expandHome(value), names)
			if err != nil {
				return "", fmt.Errorf("%s:%d: %w", path, n, err)
			}
			setIfUnset(env, k)
		case secrets[key] != "":
			setIfUnset(secrets[key], value)
		case fs.Lookup(key) != nil:
			if strings.HasSuffix(key, "-file") || key == "data" || key == "modules" || key == "push-key" || key == "ai-local-model" {
				value = expandHome(value)
			}
			if err := fs.Set(key, value); err != nil {
				return "", fmt.Errorf("%s:%d: %s: %w", path, n, key, err)
			}
		default:
			return "", fmt.Errorf("%s:%d: unknown setting %q", path, n, key)
		}
	}
	return path, sc.Err()
}

// configPath finds -config on the command line without parsing the rest.
func configPath(args []string) (path string, explicit bool) {
	for i := 0; i < len(args); i++ {
		a := strings.TrimLeft(args[i], "-")
		if len(a) == len(args[i]) || args[i] == "--" {
			break // flags end at the first non-flag
		}
		if a == "config" && i+1 < len(args) {
			return expandHome(args[i+1]), true
		}
		if v, ok := strings.CutPrefix(a, "config="); ok {
			return expandHome(v), true
		}
	}
	return defaultConfig, false
}

// readKeyFile reads an API key from a file: either the key alone, or
// shell-style lines (OPENCODE_API_KEY=..., export AW_AI_KEY=...).
func readKeyFile(path string) (string, error) {
	return readNamedKeyFile(path, []string{"AW_AI_KEY", "OPENCODE_API_KEY"})
}

func readNamedKeyFile(path string, names []string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(b))
	if !strings.Contains(text, "=") {
		if text == "" {
			return "", fmt.Errorf("%s is empty", path)
		}
		return text, nil
	}
	for _, want := range names {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimPrefix(strings.TrimSpace(line), "export ")
			if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == want {
				return unquote(strings.TrimSpace(v)), nil
			}
		}
	}
	return "", fmt.Errorf("%s has no %s= line", path, strings.Join(names, "= or "))
}

func setIfUnset(env, value string) {
	if os.Getenv(env) == "" {
		os.Setenv(env, value)
	}
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
