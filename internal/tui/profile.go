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

	"github.com/aveekpatra/cc-patcher/internal/backup"
	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// A profile records which items are on, by section, so a setup can be
// saved, shared and applied in one go on another machine.
type Profile struct {
	Version  int                 `json:"version"`
	Sections map[string][]string `json:"sections"`
	// Files carries the content of items that exist only on this machine,
	// such as your own skills, keyed by item name then relative path.
	// encoding/json stores the bytes as base64.
	Files map[string]map[string][]byte `json:"files,omitempty"`
	// Setup is the whole user-level Claude Code setup, so an import can
	// replicate it on another machine.
	Setup *backup.Backup `json:"setup,omitempty"`
}

// exporter is an item whose content travels inside the profile.
type exporter interface {
	Files() (map[string][]byte, error)
}

// InstallFiles recreates an exported item from its files; main sets it.
var InstallFiles func(name string, files map[string][]byte) error

// AutoProfilePath is where the current choices are saved after every apply.
func AutoProfilePath() string { return filepath.Join(claude.StateDir(), "profile.json") }

// DefaultExportPath is the file the home screen exports to and imports from.
func DefaultExportPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "cc-patcher-profile.json")
}

// Snapshot returns the items that are on right now. With files, it also
// carries the content of items that only exist on this machine.
func Snapshot(sections []Section, files bool) Profile {
	p := Profile{Version: 1, Sections: map[string][]string{}}
	for _, s := range sections {
		on := []string{}
		for _, it := range s.Items() {
			if _, ok := it.(launcher); ok {
				continue // launchers open other programs; nothing to replay
			}
			if it.Enabled() {
				on = append(on, it.Name())
				if ex, ok := it.(exporter); ok && files {
					if files, err := ex.Files(); err == nil && len(files) > 0 {
						if p.Files == nil {
							p.Files = map[string]map[string][]byte{}
						}
						p.Files[it.Name()] = files
					}
				}
			}
		}
		sort.Strings(on)
		p.Sections[s.Title] = on
	}
	return p
}

// Export writes the current choices and the whole Claude Code setup to
// path, or to stdout for "-". Secret-looking values are redacted unless
// secrets is true.
func Export(sections []Section, path string, secrets bool) error {
	p := Snapshot(sections, false)
	setup, err := backup.Take(secrets)
	if err != nil {
		return err
	}
	p.Setup = setup
	return write(p, path)
}

func write(p Profile, path string) error {
	b, err := json.MarshalIndent(p, "", "  ")
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
	if p.Setup != nil {
		r, err := backup.Restore(p.Setup)
		report = append(report, r...)
		if err != nil {
			report = append(report, "restore failed: "+err.Error())
			return on, off, report
		}
		claude.RepairPaths()
	}
	for _, s := range sections {
		want, ok := p.Sections[s.Title]
		if !ok {
			continue
		}
		wanted := map[string]bool{}
		for _, n := range want {
			wanted[n] = true
		}
		items := s.Items()
		have := map[string]bool{}
		for _, it := range items {
			have[it.Name()] = true
		}
		installed := false
		for _, n := range want {
			files, ok := p.Files[n]
			if have[n] || !ok || InstallFiles == nil {
				continue
			}
			if err := InstallFiles(n, files); err != nil {
				report = append(report, fmt.Sprintf("%s / %s: %v", s.Title, n, err))
				delete(wanted, n)
				continue
			}
			on++
			installed = true
		}
		if installed {
			items = s.Items() // pick up what was just installed
		}
		for _, it := range items {
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
	saveAuto(sections)
	return on, off, report
}

// saveAuto records the current choices so they survive and can be exported.
// It skips file contents to stay small and fast.
func saveAuto(sections []Section) { _ = write(Snapshot(sections, false), AutoProfilePath()) }

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}
