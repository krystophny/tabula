package web

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	jsFromImportRE       = regexp.MustCompile(`\bfrom\s+(["'])(\./[^"']+\.(?:js|mjs))(["'])`)
	jsSideEffectImportRE = regexp.MustCompile(`\bimport\s+(["'])(\./[^"']+\.(?:js|mjs))(["'])`)
	jsDynamicImportRE    = regexp.MustCompile(`\bimport\(\s*(["'])(\./[^"']+\.(?:js|mjs))(["'])\s*\)`)
	cssImportRE          = regexp.MustCompile(`url\((["']?)(\./[^"')]+\.css)(["']?)\)`)
)

func (a *App) staticAssetHandler() http.Handler {
	files := http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS())))
	if a.devRuntime {
		diskDir := filepath.Join(a.localProjectDir, "internal", "web", "static")
		files = http.StripPrefix("/static/", http.FileServer(http.Dir(diskDir)))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := staticAssetName(r.URL.Path)
		if !isVersionedStaticTextAsset(name) {
			serveStaticAssetFile(w, r, files)
			return
		}
		data, err := a.readStaticAsset(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		serveVersionedStaticTextAsset(w, name, string(data), a.bootID)
	})
}

func serveStaticAssetFile(w http.ResponseWriter, r *http.Request, files http.Handler) {
	w.Header().Set("Cache-Control", "no-store")
	files.ServeHTTP(w, r)
}

func staticAssetName(requestPath string) string {
	raw := strings.TrimPrefix(requestPath, "/static/")
	raw = strings.Trim(raw, "/")
	if raw == "" || strings.Contains(raw, "\x00") {
		return ""
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return ""
		}
	}
	clean := path.Clean("/" + raw)
	if clean == "/" {
		return ""
	}
	return strings.TrimPrefix(clean, "/")
}

func isVersionedStaticTextAsset(name string) bool {
	return strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".mjs") || strings.HasSuffix(name, ".css")
}

func (a *App) readStaticAsset(name string) ([]byte, error) {
	if a.devRuntime {
		staticDir := filepath.Join(a.localProjectDir, "internal", "web", "static")
		return os.ReadFile(filepath.Join(staticDir, filepath.FromSlash(name)))
	}
	return staticFiles.ReadFile("static/" + name)
}

func serveVersionedStaticTextAsset(w http.ResponseWriter, name, body, bootID string) {
	if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		body = versionCSSImports(body, bootID)
	} else {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		body = versionJSImports(body, bootID)
	}
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(body))
}

func versionJSImports(body, bootID string) string {
	version := url.QueryEscape(strings.TrimSpace(bootID))
	body = versionRegexpAssetURLs(jsFromImportRE, body, version)
	body = versionRegexpAssetURLs(jsSideEffectImportRE, body, version)
	return versionRegexpAssetURLs(jsDynamicImportRE, body, version)
}

func versionCSSImports(body, bootID string) string {
	version := url.QueryEscape(strings.TrimSpace(bootID))
	return versionRegexpAssetURLs(cssImportRE, body, version)
}

func versionRegexpAssetURLs(re *regexp.Regexp, body, version string) string {
	if version == "" {
		return body
	}
	return re.ReplaceAllStringFunc(body, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		versioned := assetURLWithVersion(parts[2], version)
		return strings.Replace(match, parts[2], versioned, 1)
	})
}

func assetURLWithVersion(raw, version string) string {
	if version == "" || strings.Contains(raw, "?v=") {
		return raw
	}
	base, fragment, hasFragment := strings.Cut(raw, "#")
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	versioned := base + separator + "v=" + version
	if hasFragment {
		versioned += "#" + fragment
	}
	return versioned
}
