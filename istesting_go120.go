//go:build !go1.21

package locker

func isTesting() bool {
	return false
}
