package holders

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/platform"
)

func TestManagedRuntimeClassification(t *testing.T) {
	for _, tt := range []struct {
		name, path, command string
		want                Class
	}{
		{"AS159 Homebrew runtime", "/opt/homebrew/Cellar/container/1.7.1/libexec/container-plugins/container-runtime-linux/bin/container-runtime-linux", "container-runtime-linux", ManagedRuntime},
		{"AS159 Apple VM", "/System/Library/Frameworks/Virtualization.framework/Versions/A/XPCServices/com.apple.Virtualization.VirtualMachine.xpc/Contents/MacOS/com.apple.Virtualization.VirtualMachine", "com.apple.Virtualization.VirtualMachine", ManagedRuntime},
		{"AS160 Docker Desktop VM", "/Applications/Docker.app/Contents/MacOS/com.docker.virtualization", "com.docker.virtualization", ManagedRuntime},
		{"AS161 same named user binary", "/Volumes/ExternalSSD/myapp/bin/container-runtime-linux", "container-runtime-linux", Stoppable},
		{"Apple installer plugin", "/usr/local/libexec/container/plugins/container-runtime-linux/bin/container-runtime-linux", "container-runtime-linux", ManagedRuntime},
		{"Apple installer API server", "/usr/local/bin/container-apiserver", "container-apiserver", ManagedRuntime},
		{"Homebrew keg API server", "/opt/homebrew/Cellar/container/1.7.1/cli/container-apiserver", "container-apiserver", ManagedRuntime},
		{"Homebrew opt API server", "/opt/homebrew/opt/container/bin/container-apiserver", "container-apiserver", ManagedRuntime},
		{"same named user API server", "/Volumes/ExternalSSD/myapp/bin/container-apiserver", "container-apiserver", Stoppable},
		{"similar plugin dir", "/Volumes/ExternalSSD/myapp/libexec/container-plugins-local/bin/container-runtime-linux", "container-runtime-linux", Stoppable},
		{"similar Docker bundle", "/Applications/MyDocker.app/Contents/MacOS/com.docker.virtualization", "com.docker.virtualization", Stoppable},
		{"similar macOS framework", "/System/Library/Frameworks/Virtualization.framework.local/bin/com.apple.Virtualization.VirtualMachine", "com.apple.Virtualization.VirtualMachine", Stoppable},
		{"plugin with unrelated name", "/opt/homebrew/libexec/container-plugins/custom/bin/worker", "helper", ManagedRuntime},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := platform.Holder{Name: tt.command, PID: 42, UID: 501}
			s := Service{
				PID: 100, UID: h.UID,
				Ancestors: func(int) ([]int, error) { return nil, nil },
				Inspect: func(int) (platform.Process, error) {
					return platform.Process{Name: h.Name, UID: h.UID, Path: tt.path, Started: time.Unix(10, 1)}, nil
				},
			}
			c := s.Classify([]platform.Holder{h})[0]
			if c.Class != tt.want || c.CanStop() != (tt.want == Stoppable) {
				t.Fatalf("class=%q canStop=%v, want %q", c.Class, c.CanStop(), tt.want)
			}
			if tt.want == ManagedRuntime {
				if c.Hint != "stop it through its DockGuard target or container stop" {
					t.Fatalf("hint = %q", c.Hint)
				}
				result, err := s.Stop(context.Background(), "/Volumes/ExternalSSD", c, func(string, bool) bool {
					t.Fatal("asked to stop a managed runtime")
					return false
				})
				if result.Status != NotOffered || err != nil {
					t.Fatalf("stop = %+v, %v", result, err)
				}
			}
		})
	}
}

func TestManagedRuntimeResolvesExecutableSymlink(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Docker.app", "Contents", "MacOS", "vm")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "vm")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if !isManagedRuntime(alias) {
		t.Fatal("executable alias escaped managed runtime protection")
	}
}

func TestManagedRuntimeRecheckedBeforeSignal(t *testing.T) {
	root := t.TempDir()
	ordinary := filepath.Join(root, "vm")
	runtime := filepath.Join(root, "Docker.app", "Contents", "MacOS", "vm")
	if err := os.MkdirAll(filepath.Dir(runtime), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime, nil, 0700); err != nil {
		t.Fatal(err)
	}
	h := platform.Holder{Name: "vm", PID: 42, UID: 501}
	s := Service{
		PID: 100, UID: h.UID,
		Ancestors: func(int) ([]int, error) { return nil, nil },
		Scan:      func(string) ([]platform.Holder, error) { return []platform.Holder{h}, nil },
		Inspect: func(int) (platform.Process, error) {
			return platform.Process{Name: h.Name, UID: h.UID, Path: ordinary, Started: time.Unix(10, 1)}, nil
		},
	}
	c := s.Classify([]platform.Holder{h})[0]
	if !c.CanStop() {
		t.Fatalf("initial candidate = %+v", c)
	}
	result, err := s.Stop(context.Background(), "/Volumes/ExternalSSD", c, func(string, bool) bool {
		if err := os.Symlink(runtime, ordinary); err != nil {
			t.Fatal(err)
		}
		return true
	})
	if err != nil || result.Status != NotOffered || result.Message != managedRuntimeHint {
		t.Fatalf("stop = %+v, %v", result, err)
	}
}
