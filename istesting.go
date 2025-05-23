//go:build go1.21

package locker

import "testing"

func isTesting() bool {
	return testing.Testing()
}
