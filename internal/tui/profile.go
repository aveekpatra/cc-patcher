package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// A profile records which items are on, by section, so a setup can be
// saved, shared and applied in one go on another machine.
type Profile struct {
	Version  int                 `json:"version"`
	Sections map[string][]string `json:"sections"`
}

// AutoProfilePath is where the current choices are saved after every apply.
func AutoProfilePath() string { return filepath.Join(claude.StateDir(), "profile.json") }

// DefaultExportPath is the file the home screen exports to and imports from.
func DefaultExportPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "cc-patcher-profile.json")
}

// Snapshot returns the items that are on right now.
func Snapshot(sections []Section) Profile {
	p := Profile{Version: 1, Sections: map[string][]string{}}
	for _, s := range sections {
		on := []string{}
		for _, it := range s.Items() {
			if _, ok := it.(launcher); ok {
				continue // launchers open other programs; nothing to replay
			}
			if it.Enabled() {
				on = append(on, it.Name())
			}
		}
		sort.Strings(on)
		p.Sections[s.Title] = on
	}
	return p
}

// Export writes the current choices to path, or to stdout for "-".
func Export(sections []Section, path string) error {
	b, err := json.MarshalIndent(Snapshot(sections), "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if path == "-" {
		_, err = os.Stdout.Write(b)
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(path, b, 0o644)
}

// LoadProfile reads a profile from a file path or an http(s) URL.
func LoadProfile(src string) (Profile, error) {
	var r io.Reader
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(src)
		if err != nil {
			return Profile{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return Profile{}, fmt.Errorf("%s: %s", src, resp.Status)
		}
		r = io.LimitReader(resp.Body, 1<<20)
	} else {
		f, err := os.Open(expandHome(src))
		if err != nil {
			return Profile{}, err
		}
		defer f.Close()
		r = f
	}
	var p Profile
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return Profile{}, fmt.Errorf("%s: %v", src, err)
	}
	if p.Sections == nil {
		return Profile{}, fmt.Errorf("%s: no sections in profile", src)
	}
	return p, nil
}

// Apply makes every section listed in the profile match it exactly: listed
// items are turned on, the rest of that section turned off. Sections the
// profile leaves out are not touched. It returns one line per change or
// problem.
func Apply(sections []Section, p Profile) (on, off int, report []string) {
	for _, s := range sections {
		want, ok := p.Sections[s.Title]
		if !ok {
			continue
		}
		wanted := map[string]bool{}
		for _, n := range want {
			wanted[n] = true
		}
		for _, it := range s.Items() {
			if _, ok := it.(launcher); ok {
				delete(wanted, it.Name())
				continue
			}
			name := it.Name()
			should := wanted[name]
			delete(wanted, name)
			if should == it.Enabled() {
				continue
			}
			var err error
			if should {
				err = it.Enable()
			} else {
				err = it.Disable()
			}
			switch {
			case err != nil:
				report = append(report, fmt.Sprintf("%s / %s: %v", s.Title, name, err))
			case should:
				on++
			default:
				off++
			}
		}
		for n := range wanted {
			report = append(report, fmt.Sprintf("%s / %s: not in this version, skipped", s.Title, n))
		}
	}
	sort.Strings(report)
	saveAuto(sections)
	return on, off, report
}

// saveAuto records the current choices so they survive and can be exported.
func saveAuto(sections []Section) { _ = Export(sections, AutoProfilePath()) }

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}
