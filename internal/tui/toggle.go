package tui

// Toggle is anything that can be switched on or off from a multi-select
// screen: a skill, a config option, or a patch.
type Toggle interface {
	Name() string
	Description() string
	Enabled() bool
	Enable() error
	Disable() error
}
