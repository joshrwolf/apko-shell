package script

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"chainguard.dev/apko/pkg/build/types"
	"gopkg.in/yaml.v3"
)

// Config represents the configuration extracted from a script
type Config struct {
	// Raw argument strings from #!apko-shell lines
	ShebangArgs []string

	// Parsed YAML from PEP 723 block
	ImageConfig *types.ImageConfiguration
}

// Parse reads a script and extracts configuration from shebang and PEP 723 blocks
func Parse(r io.Reader) (*Config, error) {
	cfg := &Config{}

	scanner := bufio.NewScanner(r)
	var lineNum int
	var inPEP723 bool
	var pep723Lines []string

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Handle PEP 723 block
		if strings.HasPrefix(line, "# /// apko") {
			inPEP723 = true
			continue
		}
		if inPEP723 {
			if strings.HasPrefix(line, "# ///") {
				// End of PEP 723 block, parse it
				if err := parsePEP723(strings.Join(pep723Lines, "\n"), cfg); err != nil {
					return nil, fmt.Errorf("parsing PEP 723 block: %w", err)
				}
				break
			}
			// Remove "# " prefix and collect line
			if strings.HasPrefix(line, "# ") {
				pep723Lines = append(pep723Lines, line[2:])
			} else if strings.HasPrefix(line, "#") {
				pep723Lines = append(pep723Lines, line[1:])
			}
			continue
		}

		// Parse shebang lines
		if strings.HasPrefix(line, "#!apko-shell") {
			if err := parseShebangLine(line, cfg); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			continue
		}

		// Stop parsing after first non-shebang, non-comment line
		if !strings.HasPrefix(line, "#") {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading script: %w", err)
	}

	return cfg, nil
}

func parseShebangLine(line string, cfg *Config) error {
	// Remove "#!apko-shell" prefix
	args := strings.TrimPrefix(line, "#!apko-shell")
	args = strings.TrimSpace(args)

	if args != "" {
		cfg.ShebangArgs = append(cfg.ShebangArgs, args)
	}

	return nil
}

func parsePEP723(content string, cfg *Config) error {
	var ic types.ImageConfiguration
	if err := yaml.Unmarshal([]byte(content), &ic); err != nil {
		return fmt.Errorf("unmarshaling YAML: %w", err)
	}

	cfg.ImageConfig = &ic
	return nil
}
