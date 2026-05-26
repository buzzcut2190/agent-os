package sandbox

import (
	"fmt"
	"os"
	"syscall"
)

// CreateOverlay mounts an overlayfs at merged with the given layers.
// lower is read-only base, upper is writable layer, work is a work dir.
func CreateOverlay(lower, upper, work, merged string) error {
	// Create required directories
	for _, dir := range []string{upper, work, merged} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lower, upper, work)
	if err := syscall.Mount("overlay", merged, "overlay", 0, opts); err != nil {
		return fmt.Errorf("mount overlay: %w", err)
	}
	return nil
}

// RemoveOverlay unmounts the overlayfs at merged.
func RemoveOverlay(merged string) error {
	if err := syscall.Unmount(merged, 0); err != nil {
		if err == syscall.EINVAL {
			// Not mounted
			return nil
		}
		return fmt.Errorf("unmount %s: %w", merged, err)
	}
	// Clean up the mount point directory
	os.Remove(merged)
	return nil
}
