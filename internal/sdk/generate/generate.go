// Package generate runs buf generate with a remote plugin to produce SDK
// source files from .proto inputs. The caller owns the output directory
// and must remove it when done.
package generate

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/alipourhabibi/Hades/config"
)

// Generator runs buf generate with a configured remote plugin.
type Generator struct {
	bufBin string
	config config.GeneratorConfig
}

// New creates a Generator. If bufBin is empty, "buf" is used.
func New(bufBin string, cfg config.GeneratorConfig) *Generator {
	if bufBin == "" {
		bufBin = "buf"
	}
	return &Generator{bufBin: bufBin, config: cfg}
}

// Generate runs buf generate with the configured remote plugin against protoDir
// and returns the path to the temporary directory containing the generated files.
//
// The caller owns the returned directory and must remove it when done
// (e.g. defer os.RemoveAll(outDir)). On failure the directory is removed
// before the error is returned, so the caller never sees a partial outDir.
func (g *Generator) Generate(ctx context.Context, protoDir string) (outDir string, err error) {
	outDir, err = os.MkdirTemp("", "hades-sdk-out-*")
	if err != nil {
		return "", fmt.Errorf("generate: mktemp: %w", err)
	}

	tmplDir, err := os.MkdirTemp("", "hades-sdk-tmpl-*")
	if err != nil {
		_ = os.RemoveAll(outDir)
		return "", fmt.Errorf("generate: mktemp tmpl: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmplDir) }()

	tmpl := fmt.Sprintf("version: v2\nplugins:\n  - local: %s\n    out: %s\n", g.config.Plugin, outDir)
	if g.config.Options != "" {
		tmpl += fmt.Sprintf("    opt: %s\n", g.config.Options)
	}

	tmplPath := tmplDir + "/buf.gen.yaml"
	if err := os.WriteFile(tmplPath, []byte(tmpl), 0o644); err != nil {
		_ = os.RemoveAll(outDir)
		return "", fmt.Errorf("generate: write buf.gen.yaml: %w", err)
	}

	out, execErr := exec.CommandContext(ctx, g.bufBin, "generate", "--template", tmplPath, protoDir).CombinedOutput()
	if execErr != nil {
		_ = os.RemoveAll(outDir)
		return "", fmt.Errorf("buf generate failed for %s:\n%s", g.config.Language, out)
	}

	return outDir, nil
}
