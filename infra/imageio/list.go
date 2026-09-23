package imageio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// supportedExtensions are the source image formats decode.go's registered
// decoders (image/jpeg, image/png) can read.
var supportedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

// ListDir returns the paths of files directly inside dir whose extension is
// a supported image format, sorted by name for a stable, predictable batch
// processing order.
func ListDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("imageio: reading directory %s: %w", dir, err)
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() || !supportedExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}
