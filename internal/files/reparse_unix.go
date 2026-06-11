//go:build !windows

package files

func isReparsePoint(_ string) (bool, error) {
	return false, nil
}
