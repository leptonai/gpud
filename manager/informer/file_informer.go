package informer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/manager/packages"
)

type FileInformer struct {
}

func NewFileInformer() chan packages.PackageInfo {
	i := &FileInformer{}
	return i.Start()
}

func (f *FileInformer) Start() chan packages.PackageInfo {
	c := make(chan packages.PackageInfo)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Logger.Fatal(err)
	}

	go func() {
		out, err := exec.Command("ls", "/var/lib/gpud/packages").CombinedOutput()
		if err == nil {
			for _, pkgName := range strings.Split(string(out), "\n") {
				if pkgName == "" {
					continue
				}
				scriptPath := fmt.Sprintf("/var/lib/gpud/packages/%s/init.sh", pkgName)
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
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					continue
				}
				if event.Op&fsnotify.Create == fsnotify.Create {
					fileInfo, err := os.Stat(event.Name)
					if err == nil && fileInfo.IsDir() {
						log.Logger.Infof("New directory created: %s", event.Name)
						addDirectory(watcher, event.Name)
					}
					continue
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					fileInfo, err := os.Stat(event.Name)
					if os.IsNotExist(err) || (err == nil && fileInfo.IsDir()) {
						log.Logger.Infof("Directory removed: %s", event.Name)
						watcher.Remove(event.Name)
					}
				}

				if event.Op&fsnotify.Write != fsnotify.Write {
					continue
				}
				path := event.Name
				if !strings.HasPrefix(path, "/var/lib/gpud/packages") {
					continue
				}
				elems := strings.Split(path, "/")
				if len(elems) != 7 {
					continue
				}
				fileName := elems[6]
				if fileName != "init.sh" {
					continue
				}

				version, dependencies, totalTime, err := resolvePackage(path)
				if err != nil {
					log.Logger.Errorf("resolve package failed: %v", err)
					continue
				}
				c <- packages.PackageInfo{
					Name:          elems[5],
					ScriptPath:    path,
					TargetVersion: version,
					Dependency:    dependencies,
					TotalTime:     totalTime,
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					continue
				}
				log.Logger.Errorf("Error: %s", err)
			}
		}
		watcher.Close()
	}()

	rootDir := "/var/lib/gpud/"
	err = addDirectory(watcher, rootDir)
	if err != nil {
		log.Logger.Fatal(err)
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
