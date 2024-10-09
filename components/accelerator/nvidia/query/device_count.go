package query

import (
	"os"
	"path/filepath"
	"regexp"
)

func CountAllDevicesFromDevDir() (int, error) {
	return countAllDevicesFromDir("/dev")
}

// "checkPermissions" in "nvvs/plugin_src/software/Software.cpp"
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L220-L249
func countAllDevicesFromDir(dir string) (int, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, file := range files {
		if countDevEntry(file.Name()) && canRead(filepath.Join(dir, file.Name())) {
			count++
		}
	}

	return count, nil
}

// countDevEntry checks if the entry name matches "nvidia" followed by a number
// ref. "CountDevEntry" in "nvvs/plugin_src/software/Software.cpp"
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L220-L249
func countDevEntry(entryName string) bool {
	lastElement := filepath.Base(entryName)

	// check if the entry name ends with "nvidia" followed by a number
	match, _ := regexp.MatchString(`^nvidia\d+$`, lastElement)
	return match
}

// "nvvs/plugin_src/software/Software.cpp"
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L220-L249
func canRead(file string) bool {
	f, err := os.OpenFile(file, os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	return true
}
