package pipeline

import (
	"path/filepath"

	"github.com/sarielhp/relatex/internal/artifacts"
	"github.com/sarielhp/relatex/internal/bundle"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/synctexout"
)

// publishSynctex hands the SyncTeX output the engine's PDF, the layout and source map the core
// produced, and the paper's sources through the core's artifact resolver.
func (c *conversion) publishSynctex(root string) {
	prefixes := c.prefixMap()
	synctexout.Publish(synctexout.Options{PDF: c.pdf, SourceDirectory: filepath.Join(c.probe, "external/working"),
		SourceFiles: c.builtSourceFiles(), Layout: c.synctexLayout(), SourceMap: c.sourceMap,
		Sources: func(root string, files map[string]string) synctexout.Sources {
			return synctexSources{artifacts.New(root, files, prefixes)}
		},
		Directory: root, Mode: c.options.Synctex, OriginalDirectory: c.inputDirectory()})
}

// synctexSources is the artifact resolver as the SyncTeX output reads the sources.
type synctexSources struct{ *artifacts.Resolver }

func (synctexSources) Relative(name string) string { return artifacts.Relative(name) }

func (synctexSources) AuthorBuildProduct(name, job string) bool {
	return artifacts.IsAuthorBuildProduct(name, job)
}

// synctexLayout is the layout the SyncTeX output reads: whole only when the source map has no pages
// of its own (then it takes the layout's), else the reference, whose digests it checks.
func (c *conversion) synctexLayout() rec.Map {
	if c.sourceMap == nil || c.sourceMap["pages"] == nil {
		return bundle.WholeLayout(c.layout)
	}
	return c.layout
}
