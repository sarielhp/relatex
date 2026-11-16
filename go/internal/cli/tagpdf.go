package cli

import (
	"os"
	"path/filepath"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdftag"
	"github.com/sarielhp/relatex/internal/pipeline"
	"github.com/sarielhp/relatex/internal/rec"
)

// tagPDFLong is the tag-pdf command's help (decision 105, the route-2 prototype).
const tagPDFLong = "Tags the ordinary PDF of a finished conversion from relatex's model (PDF/UA-2, route 2): structure from document.json.gz, content bound through the layout parts. Writes OUTPUT_DIRECTORY/pdf-ua2/JOB.pdf and export.json, validated by veraPDF when installed. --web-report FILE is the accessibility.json of the same document, from an earlier conversion: the web claim the tagged PDF's PDF/UA-2 identification is judged against. The identification stays in the file's metadata only when the file earns it."

// tagPDFSpec is relatex tag-pdf.
func tagPDFSpec() commandSpec {
	var pdf, target, webReport string
	options := []clihelp.Option{
		textFlag("--pdf <file>", "The ordinary PDF to tag (default: the conversion's own)", &pdf),
		textFlag("--target <file>", "Where the tagged PDF is written", &target),
		textFlag("--web-report <file>", "accessibility.json of the same document, from an earlier conversion", &webReport),
	}
	return commandSpec{name: "tag-pdf", description: "Tag the ordinary PDF of a finished conversion from relatex's model (PDF/UA-2)",
		long: tagPDFLong, usage: Name + " tag-pdf [OUTPUT_DIRECTORY] [--pdf FILE] [--target FILE] [--web-report FILE]", options: options,
		args: clihelp.MaximumNArgs(1), human: tagPDFHuman, run: func(rest []string) ojson.Object { return tagPDFCommand(rest, pdf, target, webReport) }}
}

// tagPDFCommand is relatex tag-pdf.
func tagPDFCommand(rest []string, pdf, target, webReport string) ojson.Object {
	options := pdftag.Options{Glyphs: filepath.Join(Root, "tex", "relatex-symbol-glyphs.txt")}
	if pdf != "" {
		options.PDF = expandPath(pdf)
	}
	if target != "" {
		options.Target = expandPath(target)
	}
	if webReport != "" {
		webReport = expandPath(webReport)
	}
	options.Output = lastProduct(".", nil)
	if len(rest) == 1 {
		options.Output = rest[0]
	}
	options.Output = expandPath(options.Output)
	result := pipeline.TagOrdinary(options)
	if result.Get("status") == "failed" {
		rec.Fail("tag-pdf: %v", result.Get("reason"))
	}
	result = tagPDFClaim(result, options.Output, webReport)
	tagging, _ := result.Get("tagging").(ojson.Object)
	path := filepath.Join(filepath.Dir(rec.ToS(tagging.Get("pdf"))), "export.json")
	if err := os.WriteFile(path, append(ojson.Pretty(result), '\n'), 0o644); err != nil {
		panic(systemCall(err))
	}
	return result
}

// tagPDFClaim keeps the tagged PDF's PDF/UA-2 identification only when the file earns it (decision
// 112): TagOrdinary writes it whenever pdftag tags, so the same claim convert's pdf-ua stage uses is
// judged here. The web claim is the export root's own accessibility.json, else --web-report, else "".
func tagPDFClaim(result ojson.Object, root, webReport string) ojson.Object {
	tagging, _ := result.Get("tagging").(ojson.Object)
	return pipeline.EarnedClaim(result, rec.ToS(tagging.Get("pdf")), pipeline.WebClaim(root, webReport))
}
