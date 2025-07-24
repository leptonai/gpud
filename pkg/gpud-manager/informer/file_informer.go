package informer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/log"
)

type fileInformer struct {
	packagesDir string
	rootDir     string
}

func NewFileInformer() chan packages.PackageInfo {
	i := &fileInformer{
		packagesDir: "/var/lib/gpud/packages",
		rootDir:     "/var/lib/gpud/",
	}
	return i.Start()
}

func NewFileInformerWithConfig(packagesDir, rootDir string) chan packages.PackageInfo {
	i := &fileInformer{
		packagesDir: packagesDir,
		rootDir:     rootDir,
	}
	return i.Start()
}

func (f *fileInformer) listPackages() ([]byte, error) {
	return exec.Command("ls", f.packagesDir).CombinedOutput()
}

func (f *fileInformer) processInitialPackages(c chan packages.PackageInfo) {
	out, err := f.listPackages()
	if err == nil {
		for _, pkgName := range strings.Split(string(out), "\n") {
			if pkgName == "" {
				continue
			}
			scriptPath := filepath.Join(f.packagesDir, pkgName, "init.sh")
			version, dependencies, totalTime, err := resolvePackage(scriptPath)
			if err != nil {
				log.Logger.Errorf("resolve package failed: %v", err)
				continue
			}

			c <- packages.PackageInfo{
				Name:          pkgName,
				ScriptPath:    scriptPath,
				TargetVersion: version,
				Dependency:    dependencies,
				TotalTime:     totalTime,
			}
		}
	}
}

func (f *fileInformer) handleWatcherEvents(watcher *fsnotify.Watcher, c chan packages.PackageInfo) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				continue
			}
			f.handleFileEvent(watcher, event, c)
		case wErr, ok := <-watcher.Errors:
			if !ok {
				continue
			}
			log.Logger.Errorf("Error: %s", wErr)
		}
	}
}

func (f *fileInformer) handleFileEvent(watcher interface{}, event fsnotify.Event, c chan packages.PackageInfo) {
	if event.Op&fsnotify.Create == fsnotify.Create {
		fileInfo, err := os.Stat(event.Name)
		if err == nil && fileInfo.IsDir() {
			log.Logger.Infof("New directory created: %s", event.Name)
			if w, ok := watcher.(*fsnotify.Watcher); ok {
				if aErr := addDirectory(w, event.Name); aErr != nil {
					log.Logger.Error(aErr)
				}
			}
		}
		return
	}
	if event.Op&fsnotify.Remove == fsnotify.Remove {
		fileInfo, err := os.Stat(event.Name)
		if os.IsNotExist(err) || (err == nil && fileInfo.IsDir()) {
			if w, ok := watcher.(*fsnotify.Watcher); ok {
				if rErr := w.Remove(event.Name); rErr != nil {
					log.Logger.Debug(rErr)
				}
			}
		}
	}

	if event.Op&fsnotify.Write != fsnotify.Write {
		return
	}
	path := event.Name
	if !strings.HasPrefix(path, f.packagesDir) {
		return
	}
	elems := strings.Split(path, "/")
	// Calculate expected number of elements: packagesDir elements + packageName + init.sh
	packagesElems := strings.Split(f.packagesDir, "/")
	expectedElems := len(packagesElems) + 2 // +1 for package name, +1 for init.sh
	if len(elems) != expectedElems {
		return
	}
	fileName := elems[len(elems)-1] // Last element should be init.sh
	if fileName != "init.sh" {
		return
	}

	version, dependencies, totalTime, err := resolvePackage(path)
	if err != nil {
		log.Logger.Errorf("resolve package failed: %v", err)
		return
	}
	c <- packages.PackageInfo{
		Name:          elems[len(elems)-2], // Second-to-last element is the package name
		ScriptPath:    path,
		TargetVersion: version,
		Dependency:    dependencies,
		TotalTime:     totalTime,
	}
}

func (f *fileInformer) Start() chan packages.PackageInfo {
	c := make(chan packages.PackageInfo)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Logger.Fatal(err)
	}

	go func() {
		defer func() {
			if err = watcher.Close(); err != nil {
				log.Logger.Error(err)
			}
		}()
		f.processInitialPackages(c)
		f.handleWatcherEvents(watcher, c)
	}()

	err = addDirectory(watcher, f.rootDir)
	if err != nil {
		log.Logger.Error(err)
	}
	return c
}

func addDirectory(watcher *fsnotify.Watcher, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			log.Logger.Infof("Watching directory: %s", path)
			if err := watcher.Add(path); err != nil {
				return err
			}
		}
		return nil
	})
}

func resolvePackage(path string) (string, [][]string, time.Duration, error) {
	var version string
	var dependencies [][]string
	var totalTime time.Duration
	if _, err := exec.Command("stat", path).CombinedOutput(); err != nil {
		return "", nil, 0, fmt.Errorf("stat failed: %v", err)
	}
	if out, err := exec.Command("bash", "-c", fmt.Sprintf("grep \"#GPUD_PACKAGE_VERSION\" %s | awk -F \"=\"  '{print $2}'", path)).CombinedOutput(); err != nil {
		return "", nil, 0, fmt.Errorf("get version failed: %v output: %s", err, out)
	} else {
		version = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("bash", "-c", fmt.Sprintf("grep \"#GPUD_PACKAGE_DEPENDENCY\" %s | awk -F \"=\"  '{print $2}'", path)).CombinedOutput(); err != nil {
		return "", nil, 0, fmt.Errorf("get dependencies failed: %v output: %s", err, out)
	} else {
		dependencies = resolveDependencies(string(out))
	}
	if out, err := exec.Command("bash", "-c", fmt.Sprintf("grep \"#GPUD_PACKAGE_INSTALL_TIME\" %s | awk -F \"=\"  '{print $2}'", path)).CombinedOutput(); err != nil {
		return "", nil, 0, fmt.Errorf("get dependencies failed: %v output: %s", err, out)
	} else {
		totalTime, _ = time.ParseDuration(strings.TrimSpace(string(out)))
	}
	return version, dependencies, totalTime, nil
}

func resolveDependencies(raw string) [][]string {
	raw = strings.TrimSpace(raw)
	var dependencies [][]string
	rawDependencies := strings.Split(raw, ",")
	for _, rawDependency := range rawDependencies {
		dependency := strings.Split(rawDependency, ":")
		if len(dependency) != 2 {
			continue
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies
}
