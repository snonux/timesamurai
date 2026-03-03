package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkLoginStatusLogoutFlow(t *testing.T) {
	dbDir := t.TempDir()
	cfgPath := writeWorkConfig(t, dbDir, "host-a")

	out, err := runRootCommand("--config", cfgPath, "work", "login", "--at", "2026-01-05T09:00")
	if err != nil {
		t.Fatalf("work login error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "Logged in:") {
		t.Fatalf("unexpected login output: %q", out)
	}

	out, err = runRootCommand("--config", cfgPath, "work", "status")
	if err != nil {
		t.Fatalf("work status error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "Logged in: work") {
		t.Fatalf("unexpected status output after login: %q", out)
	}

	out, err = runRootCommand("--config", cfgPath, "work", "logout", "--at", "2026-01-05T18:00")
	if err != nil {
		t.Fatalf("work logout error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "Logged out:") {
		t.Fatalf("unexpected logout output: %q", out)
	}

	out, err = runRootCommand("--config", cfgPath, "work", "status")
	if err != nil {
		t.Fatalf("work status error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "Not logged in.") {
		t.Fatalf("unexpected status output after logout: %q", out)
	}
}

func TestWorkAddSubUseBufferAndReport(t *testing.T) {
	dbDir := t.TempDir()
	cfgPath := writeWorkConfig(t, dbDir, "host-b")

	out, err := runRootCommand("--config", cfgPath, "work", "add", "1h", "--at", "2026-01-06T10:00")
	if err != nil {
		t.Fatalf("work add error = %v (output: %q)", err, out)
	}
	out, err = runRootCommand("--config", cfgPath, "work", "sub", "30m", "--at", "2026-01-06T11:00")
	if err != nil {
		t.Fatalf("work sub error = %v (output: %q)", err, out)
	}
	out, err = runRootCommand("--config", cfgPath, "work", "use-buffer", "15m", "--at", "2026-01-06T12:00")
	if err != nil {
		t.Fatalf("work use-buffer error = %v (output: %q)", err, out)
	}

	out, err = runRootCommand("--config", cfgPath, "work", "report", "--no-color")
	if err != nil {
		t.Fatalf("work report error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "work:") {
		t.Fatalf("work report missing work field: %q", out)
	}
	if !strings.Contains(out, "buffer:") {
		t.Fatalf("work report missing buffer field: %q", out)
	}
}

func writeWorkConfig(t *testing.T, dbDir, host string) string {
	t.Helper()

	content := `{
  "worktime_db_dir": "` + dbDir + `",
  "hostname": "` + host + `"
}
`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func runRootCommand(args ...string) (string, error) {
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}
