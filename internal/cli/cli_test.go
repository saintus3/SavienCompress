package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestArguments(t *testing.T) {
	tests := []struct {
		args []string
		code int
	}{
		{nil, 2},
		{[]string{"--help"}, 0},
		{[]string{"unknown"}, 2},
		{[]string{"compress", "--help"}, 0},
		{[]string{"extract", "--help"}, 0},
		{[]string{"list", "--help"}, 0},
		{[]string{"compress", "input"}, 2},
		{[]string{"compress", "input", "-o"}, 2},
		{[]string{"extract", "input", "-o", ""}, 2},
		{[]string{"list", "one", "two"}, 2},
		{[]string{"list", "input", "--overwrite"}, 2},
		{[]string{"list", "input", "--verbose=invalid"}, 2},
		{[]string{"compress", "missing", "-o", "output.zip"}, 1},
		{[]string{"list", "missing.zip"}, 1},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(tt.args, &stdout, &stderr); code != tt.code {
				t.Fatalf("exit = %d, want %d; stderr: %s", code, tt.code, &stderr)
			}
			if tt.code == 0 && !strings.Contains(stdout.String(), "Usage: savien") {
				t.Fatalf("missing help: %s", &stdout)
			}
			if tt.code != 0 && stderr.Len() == 0 {
				t.Fatal("missing error output")
			}
		})
	}
}
