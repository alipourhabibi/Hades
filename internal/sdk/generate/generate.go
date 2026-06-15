// Package generate runs protoc with a configured plugin to produce SDK
// source files from .proto inputs. The caller owns the output directory
// and must remove it when done.
package generate

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/alipourhabibi/Hades/config"
)

// Generator runs protoc with a configured plugin.
type Generator struct {
	protocBin string
	config    config.GeneratorConfig
}

// New creates a Generator. If protocBin is empty, "protoc" is used.
func New(protocBin string, cfg config.GeneratorConfig) *Generator {
	if protocBin == "" {
		protocBin = "protoc"
	}
	return &Generator{protocBin: protocBin, config: cfg}
}

// Generate runs protoc with the configured plugin against protoDir and
// returns the path to the temporary directory containing the generated files.
//
// The caller owns the returned directory and must remove it when done
// (e.g. defer os.RemoveAll(outDir)). On failure the directory is removed
// before the error is returned, so the caller never sees a partial outDir.
func (g *Generator) Generate(ctx context.Context, protoDir string) (outDir string, err error) {
	outDir, err = os.MkdirTemp("", "hades-sdk-out-*")
	if err != nil {
		return "", fmt.Errorf("generate: mktemp: %w", err)
	}

	args := buildProtocArgs(g.config.Plugin, g.config.Options, protoDir, outDir)
	out, execErr := exec.CommandContext(ctx, g.protocBin, args...).CombinedOutput()
	if execErr != nil {
		_ = os.RemoveAll(outDir)
		return "", fmt.Errorf("protoc failed for %s:\n%s", g.config.Language, out)
	}

	return outDir, nil
}

// buildProtocArgs constructs the protoc argument list.
func buildProtocArgs(plugin, options, protoDir, outDir string) []string {
	var protos []string
	_ = filepath.WalkDir(protoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".proto" {
			protos = append(protos, path)
		}
		return nil
	})

	optStr := outDir
	if options != "" {
		optStr = options + ":" + outDir
	}

	args := []string{
		fmt.Sprintf("--%s_out=%s", plugin, optStr),
		"-I" + protoDir,
	}
	return append(args, protos...)
}
