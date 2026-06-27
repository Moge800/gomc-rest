package main

import "testing"

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
