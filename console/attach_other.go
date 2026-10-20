//go:build !windows

package console

// Attach does nothing: other systems always have their std handles.
func Attach() bool {
	return true
}
