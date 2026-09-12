// Package generate runs buf generate with a remote plugin to produce SDK
// source files from .proto inputs. The caller owns the output directory
// and must remove it when done.
package generate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/alipourhabibi/Hades/config"
	"gopkg.in/yaml.v3"
)

// DefaultTimeout bounds a single generation run.
//
// It must be shorter than the worker's stale-job recovery window, otherwise a
// job that legitimately runs long is reclaimed and generated a second time
// while the first is still going.
const DefaultTimeout = 3 * time.Minute

// pluginNamePattern is what a local plugin name may look like. It exists to
// keep a value with a newline in it out of the generated buf.gen.yaml, which
// would otherwise let that value inject arbitrary configuration keys. The YAML
// is marshalled properly as well; this is the second of the two.
var pluginNamePattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// Generator runs buf generate with a configured remote plugin.
type Generator struct {
	bufBin  string
	config  config.GeneratorConfig
	timeout time.Duration
}

// New creates a Generator. If bufBin is empty, "buf" is used.
func New(bufBin string, cfg config.GeneratorConfig) *Generator {
	if bufBin == "" {
		bufBin = "buf"
	}
	return &Generator{bufBin: bufBin, config: cfg, timeout: DefaultTimeout}
}

// WithTimeout overrides the per-run timeout.
func (g *Generator) WithTimeout(d time.Duration) *Generator {
	if d > 0 {
		g.timeout = d
	}
	return g
}

// bufGenTemplate is the buf.gen.yaml document, marshalled rather than
// interpolated.
type bufGenTemplate struct {
	Version string            `yaml:"version"`
	Plugins []bufGenTemplateP `yaml:"plugins"`
}

type bufGenTemplateP struct {
	Local string `yaml:"local"`
	Out   string `yaml:"out"`
	Opt   string `yaml:"opt,omitempty"`
}

// Generate runs buf generate with the configured remote plugin against protoDir
// and returns the path to the temporary directory containing the generated files.
//
// options overrides the configured plugin options when non-empty, so a job that
// recorded its own options actually uses them. Pass "" to use the configured
// value.
//
// The caller owns the returned directory and must remove it when done
// (e.g. defer os.RemoveAll(outDir)). On failure the directory is removed
// before the error is returned, so the caller never sees a partial outDir.
func (g *Generator) Generate(ctx context.Context, protoDir string, options string) (outDir string, err error) {
	if !pluginNamePattern.MatchString(g.config.Plugin) {
		return "", fmt.Errorf("generate: plugin name %q is not a valid local plugin name", g.config.Plugin)
	}
	opt := g.config.Options
	if options != "" {
		opt = options
	}

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

	doc, err := yaml.Marshal(bufGenTemplate{
		Version: "v2",
		Plugins: []bufGenTemplateP{{Local: g.config.Plugin, Out: outDir, Opt: opt}},
	})
	if err != nil {
		_ = os.RemoveAll(outDir)
		return "", fmt.Errorf("generate: marshal buf.gen.yaml: %w", err)
	}

	tmplPath := filepath.Join(tmplDir, "buf.gen.yaml")
	if err := os.WriteFile(tmplPath, doc, 0o600); err != nil {
		_ = os.RemoveAll(outDir)
		return "", fmt.Errorf("generate: write buf.gen.yaml: %w", err)
	}

	// Each run gets its own deadline rather than inheriting the worker's
	// process-lifetime context. Without one, a hung buf invocation ran until
	// the process exited while stale-job recovery started a second copy.
	runCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	// #nosec G204 -- the binary is sdk.bufBin from configuration; the plugin name is checked against pluginNamePattern above.
	out, execErr := exec.CommandContext(runCtx, g.bufBin, "generate", "--template", tmplPath, protoDir).CombinedOutput()
	if execErr != nil {
		_ = os.RemoveAll(outDir)
		// execErr is wrapped, not dropped. Keeping only the captured output
		// meant a failure such as "buf: executable file not found" surfaced as
		// an empty message, because buf never ran and so never printed.
		return "", fmt.Errorf("buf generate failed for %s: %w\n%s", g.config.Language, execErr, out)
	}

	return outDir, nil
}
