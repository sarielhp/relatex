package cli

import "testing"

func convertOptions(t *testing.T, arguments ...string) map[string]any {
	t.Helper()
	options, _ := parsedConvert(t, arguments...)
	return options
}

// --all selects every export format but the separate builds (the tagged PDF), replacing the
// configured selection; --pdf-ua adds the tagged PDF.
func TestAllSelectsEveryFormat(t *testing.T) {
	formats, _ := convertOptions(t, "--all")["formats"].([]any)
	if len(formats) != len(exportFormats)-len(separateBuilds) {
		t.Fatalf("formats %v", formats)
	}
	for index, format := range exportFormats[:len(formats)] {
		if formats[index] != format {
			t.Errorf("format %d is %v, want %s", index, formats[index], format)
		}
	}
	for _, format := range formats {
		if format == "pdf-ua" {
			t.Fatal("--all builds the tagged PDF")
		}
	}
	extra, _ := convertOptions(t, "--pdf-ua")["extra_formats"].([]any)
	if len(extra) != 1 || extra[0] != "pdf-ua" {
		t.Fatalf("--pdf-ua: %v", extra)
	}
}

// --accessible is the accessibility conversion (decision 112): HTML, EPUB and the tagged PDF, the
// browser measurement and the report page; --all with it still adds the tagged PDF, which --all
// alone never builds.
func TestAccessibleTurnsOnTheAccessibilityConversion(t *testing.T) {
	options := convertOptions(t, "--all", "--accessible")
	extra, _ := options["extra_formats"].([]any)
	if len(extra) != 3 || extra[0] != "html" || extra[1] != "epub" || extra[2] != "pdf-ua" {
		t.Fatalf("--accessible formats: %v", extra)
	}
	if options["verify_browser"] != true || options["accessible"] != true {
		t.Fatalf("--accessible options: %v", options)
	}
	formats, _ := options["formats"].([]any)
	if all := union(formats, extra); len(all) != len(exportFormats) {
		t.Fatalf("--all --accessible: %v", all)
	}
}

// The layout viewer is a development aid: only --layout-viewer writes it.
func TestLayoutViewerIsOptIn(t *testing.T) {
	if _, set := convertOptions(t)["layout_viewer"]; set {
		t.Fatal("layout viewer on by default")
	}
	if viewer, _ := convertOptions(t, "--layout-viewer")["layout_viewer"].(bool); !viewer {
		t.Fatal("--layout-viewer not recorded")
	}
}
