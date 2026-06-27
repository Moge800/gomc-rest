package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseConfigToken(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"unset is empty", nil, nil, ""},
		{"from env", nil, map[string]string{"GOMCR_TOKEN": "envtok"}, "envtok"},
		{"flag only", []string{"-token", "flagtok"}, nil, "flagtok"},
		{"flag overrides env", []string{"-token", "flagtok"}, map[string]string{"GOMCR_TOKEN": "envtok"}, "flagtok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) string { return tc.env[k] }
			cfg, err := parseConfig(tc.args, lookup, nil)
			if err != nil {
				t.Fatalf("parseConfig: %v", err)
			}
			if cfg.Token != tc.want {
				t.Errorf("Token = %q, want %q", cfg.Token, tc.want)
			}
		})
	}
}

// TestParseConfigHelpHidesToken guards against GOMCR_TOKEN leaking into the
// -h help output (it must not be used as the flag's default value).
func TestParseConfigHelpHidesToken(t *testing.T) {
	const secret = "super-secret-token"
	lookup := func(k string) string {
		if k == "GOMCR_TOKEN" {
			return secret
		}
		return ""
	}
	var out bytes.Buffer
	_, err := parseConfig([]string{"-h"}, lookup, &out)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
	if strings.Contains(out.String(), secret) {
		t.Errorf("help output leaked the token value:\n%s", out.String())
	}
}
