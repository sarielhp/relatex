package cli

import (
	"net/url"
	"os"
	"strings"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// describerSettings are the describing service's settings in alt_text (endpoint, models,
// key_file), handed to the command as RELATEX_DESCRIBER_* variables. relatex never opens the key
// file (decision 086: no keys in relatex); it only checks that the file exists and that nobody but
// its owner may read it, so the configuration can be shared while the key stays in its own file.
type describerSettings struct {
	endpoint string
	models   []string
	keyFile  string
}

func readDescriberSettings(section ojson.Object) describerSettings {
	result := describerSettings{}
	result.endpoint, _ = section.Get("endpoint").(string)
	list, _ := section.Get("models").([]any)
	for _, item := range list {
		result.models = append(result.models, rec.AsString(item))
	}
	if file, ok := section.Get("key_file").(string); ok && file != "" {
		result.keyFile = expandPath(file)
	}
	return result
}

// environment is the RELATEX_DESCRIBER_* variables of the settings that are set.
func (s describerSettings) environment() []string {
	var env []string
	if s.endpoint != "" {
		env = append(env, "RELATEX_DESCRIBER_ENDPOINT="+s.endpoint)
	}
	if len(s.models) > 0 {
		env = append(env, "RELATEX_DESCRIBER_MODELS="+strings.Join(s.models, ","))
	}
	if s.keyFile != "" {
		env = append(env, "RELATEX_DESCRIBER_KEY_FILE="+s.keyFile)
	}
	return env
}

// checkKeyFile stops a draft whose key file is missing or readable by others; it stats the file
// and never opens it.
func (s describerSettings) checkKeyFile() {
	if s.keyFile == "" {
		return
	}
	info, err := os.Stat(s.keyFile)
	if err != nil {
		rec.Fail("alt_text.key_file %s does not exist: create it with the API key alone, then %s", s.keyFile, keyFileRemedy(s.keyFile))
	}
	if exposure := keyFileExposure(s.keyFile, info); exposure != "" {
		rec.Fail("alt_text.key_file %s %s: %s", s.keyFile, exposure, keyFileRemedy(s.keyFile))
	}
}

func httpURL(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	parsed, err := url.Parse(text)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && text != ""
}
