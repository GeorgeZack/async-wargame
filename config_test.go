package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "api-key.env")
	os.WriteFile(keyFile, []byte("# OpenCode\nexport OPENCODE_API_KEY=\"sk-test\"\n"), 0o600)
	conf := filepath.Join(dir, "aw.conf")
	os.WriteFile(conf, []byte(`# test
addr = 0.0.0.0:9000
proxy = true
ai = opencode
ai-key-file = `+keyFile+`
host-password = "hunter2"
`), 0o600)
	t.Setenv("AW_AI_KEY", "")
	t.Setenv("AW_HOST_PASSWORD", "")
	os.Unsetenv("AW_AI_KEY")
	os.Unsetenv("AW_HOST_PASSWORD")

	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "")
	proxy := fs.Bool("proxy", false, "")
	ai := fs.String("ai", "", "")
	fs.String("config", "", "")
	args := []string{"-config", conf, "-addr", ":7000"}
	if _, err := loadConfig(fs, args); err != nil {
		t.Fatal(err)
	}
	fs.Parse(args)
	if *addr != ":7000" || !*proxy || *ai != "opencode" {
		t.Errorf("addr %q proxy %v ai %q", *addr, *proxy, *ai)
	}
	if os.Getenv("AW_AI_KEY") != "sk-test" || os.Getenv("AW_HOST_PASSWORD") != "hunter2" {
		t.Errorf("secrets: key %q password %q", os.Getenv("AW_AI_KEY"), os.Getenv("AW_HOST_PASSWORD"))
	}

	// Mistakes are reported with the line.
	os.WriteFile(conf, []byte("adr = :1\n"), 0o600)
	if _, err := loadConfig(flag.NewFlagSet("t", flag.ContinueOnError), []string{"-config", conf}); err == nil || !strings.Contains(err.Error(), ":1: unknown setting") {
		t.Errorf("typo: %v", err)
	}
	// No file and none asked for is fine; one asked for must exist.
	t.Chdir(dir)
	if p, err := loadConfig(fs, nil); p != "" || err != nil {
		t.Errorf("no default file: %q %v", p, err)
	}
	if _, err := loadConfig(fs, []string{"--config=missing.conf"}); err == nil {
		t.Error("a missing -config file was ignored")
	}
}

func TestAIInstructionText(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "in.txt")
	os.WriteFile(f, []byte("\nKeep outcomes under 80 words.\n"), 0o600)
	got, err := aiInstructionText(`Answer directly.\nDon't reason at length.`, f)
	want := "Answer directly.\nDon't reason at length.\n\nKeep outcomes under 80 words."
	if err != nil || got != want {
		t.Errorf("got %q, %v", got, err)
	}
	if got, _ := aiInstructionText("", ""); got != "" {
		t.Errorf("empty: %q", got)
	}
	if _, err := aiInstructionText("", filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing file was ignored")
	}
	// The config file sets it like any flag.
	conf := filepath.Join(dir, "aw.conf")
	os.WriteFile(conf, []byte("ai-instructions = Answer directly. Keep it short.\n"), 0o600)
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	in := fs.String("ai-instructions", "", "")
	fs.String("config", "", "")
	if _, err := loadConfig(fs, []string{"-config", conf}); err != nil || *in != "Answer directly. Keep it short." {
		t.Errorf("from config: %q, %v", *in, err)
	}
}
