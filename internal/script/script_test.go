package script

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		wantArgs    []string
		wantHasYAML bool
		wantErr     bool
	}{
		{
			name: "simple shebang with packages",
			script: `#!/usr/bin/env apko-shell
#!apko-shell -p curl,jq
echo "hello"`,
			wantArgs: []string{"-p curl,jq"},
		},
		{
			name: "multiple shebang lines",
			script: `#!/usr/bin/env apko-shell
#!apko-shell -p curl
#!apko-shell -p jq
echo "hello"`,
			wantArgs: []string{"-p curl", "-p jq"},
		},
		{
			name:     "empty script",
			script:   `#!/usr/bin/env apko-shell`,
			wantArgs: nil,
		},
		{
			name: "PEP 723 block",
			script: `#!/usr/bin/env apko-shell
# /// apko
# contents:
#   repositories:
#     - https://packages.wolfi.dev/os
#   packages:
#     - wolfi-base
#     - python3
# cmd: /usr/bin/python3
# ///
print("hello")`,
			wantHasYAML: true,
		},
		{
			name: "shebang and PEP 723",
			script: `#!/usr/bin/env apko-shell
#!apko-shell -p curl,jq
# /// apko
# contents:
#   packages:
#     - wolfi-base
#     - python3
# cmd: /usr/bin/python3
# ///
print("hello")`,
			wantArgs:    []string{"-p curl,jq"},
			wantHasYAML: true,
		},
		{
			name: "complex shebang flags",
			script: `#!/usr/bin/env apko-shell
#!apko-shell -p curl,jq --shell=/bin/bash
#!apko-shell --repository https://packages.wolfi.dev/os
echo "hello"`,
			wantArgs: []string{
				"-p curl,jq --shell=/bin/bash",
				"--repository https://packages.wolfi.dev/os",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(strings.NewReader(tt.script))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check shebang args
			if len(cfg.ShebangArgs) != len(tt.wantArgs) {
				t.Errorf("Parse() got %d args, want %d", len(cfg.ShebangArgs), len(tt.wantArgs))
			}
			for i, arg := range cfg.ShebangArgs {
				if i < len(tt.wantArgs) && arg != tt.wantArgs[i] {
					t.Errorf("Parse() arg[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}

			// Check if YAML was parsed
			if tt.wantHasYAML && cfg.ImageConfig == nil {
				t.Errorf("Parse() ImageConfig = nil, want non-nil")
			}
			if !tt.wantHasYAML && cfg.ImageConfig != nil {
				t.Errorf("Parse() ImageConfig = non-nil, want nil")
			}
		})
	}
}
