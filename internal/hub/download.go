package hub

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/stranix79/deckhand/internal/version"
)

const releasesURL = "https://github.com/stranix79/deckhand/releases"

var releaseVersion = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)$`)

// downloadAsset is one release file offered on the download page.
type downloadAsset struct{ Label, URL string }

// downloadPage is /download: the one-line installer, then the manual ways,
// system by system. The hub is deployed from a release tag, so its own
// version is the latest release and gives direct links to the archives; a
// development build falls back to the releases page.
func (h *Hub) downloadPage(w http.ResponseWriter, r *http.Request, lang string) {
	title, desc := "Download", "Install Deckhand in one line on macOS and Linux, or take the binary for Windows. Free and open source, one file, no account."
	if lang == "fr" {
		title, desc = "Installer", "Installez Deckhand en une ligne sur macOS et Linux, ou prenez le binaire pour Windows. Libre et gratuit, un seul fichier, pas de compte."
	}
	path := func(l string) string {
		if l == "fr" {
			return "/download/fr"
		}
		return "/download"
	}
	other := "fr"
	if lang == "fr" {
		other = "en"
	}

	ver := ""
	if m := releaseVersion.FindStringSubmatch(version.Version); m != nil {
		ver = m[1]
	}
	asset := func(label, suffix string) downloadAsset {
		if ver == "" {
			return downloadAsset{label, releasesURL + "/latest"}
		}
		return downloadAsset{label, releasesURL + "/download/v" + ver + "/deckhand_" + ver + "_" + suffix}
	}
	h.renderSite(w, r, "download.html", map[string]any{
		"Title": title, "Lang": lang, "Nav": "download", "Wide": true,
		"Alternates": []blogAlternate{{lang, h.cfg.BaseURL + path(lang)}, {other, h.cfg.BaseURL + path(other)}},
		"LangEN":     path("en"), "LangFR": path("fr"),
		"SEO":      seo{Description: desc, Canonical: h.cfg.BaseURL + path(lang)},
		"Version":  ver,
		"Releases": releasesURL,
		"Install":  "curl -fsSL " + strings.TrimSuffix(h.cfg.BaseURL, "/") + "/install.sh | sh",
		"Mac":      []downloadAsset{asset("Apple silicon", "darwin_arm64.tar.gz"), asset("Intel", "darwin_amd64.tar.gz")},
		"Linux":    []downloadAsset{asset("x86_64", "linux_amd64.tar.gz"), asset("arm64", "linux_arm64.tar.gz")},
		"Windows":  asset("x86_64", "windows_amd64.zip"),
	})
}
