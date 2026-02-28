package p9

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bd "github.com/steveyegge/beads"
)

func TestMountUnmount(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")
	
	store, err := bd.OpenFromConfig(context.Background(), filepath.Join(tmpDir, "main"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	beadsFS := NewBeadsFS(store, context.Background())

	projectDir := filepath.Join(tmpDir, "project1")
	os.MkdirAll(projectDir, 0755)

	err = beadsFS.Mount("proj1", projectDir)
	if err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	mounts := beadsFS.ListMounts()
	if _, ok := mounts["proj1"]; !ok {
		t.Fatal("Mount not listed")
	}

	err = beadsFS.Umount("proj1")
	if err != nil {
		t.Fatalf("Umount failed: %v", err)
	}

	mounts = beadsFS.ListMounts()
	if _, ok := mounts["proj1"]; ok {
		t.Fatal("Mount still listed after umount")
	}
}

func TestMountReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")
	
	store, err := bd.OpenFromConfig(context.Background(), filepath.Join(tmpDir, "main"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	beadsFS := NewBeadsFS(store, context.Background())

	projectDir := filepath.Join(tmpDir, "project1")
	os.MkdirAll(projectDir, 0755)

	err = beadsFS.Mount("proj1", projectDir)
	if err != nil {
		t.Fatal(err)
	}
	defer beadsFS.Umount("proj1")

	err = beadsFS.Write("proj1/ctl", []byte("init bd"), "test")
	if err != nil {
		t.Fatal(err)
	}

	err = beadsFS.Write("proj1/ctl", []byte("new 'Test Task' 'Description'"), "test")
	if err != nil {
		t.Fatal(err)
	}

	list, err := beadsFS.Read("proj1/list")
	if err != nil {
		t.Fatal(err)
	}

	var beads []map[string]interface{}
	json.Unmarshal(list, &beads)
	if len(beads) != 1 {
		t.Fatalf("Expected 1 bead, got %d", len(beads))
	}

	beadID := beads[0]["id"].(string)
	err = beadsFS.Write("proj1/ctl", []byte("label "+beadID+" claimable"), "test")
	if err != nil {
		t.Fatal(err)
	}

	ready, err := beadsFS.Read("proj1/ready")
	if err != nil {
		t.Fatal(err)
	}

	var readyBeads []map[string]interface{}
	json.Unmarshal(ready, &readyBeads)
	if len(readyBeads) != 1 {
		t.Fatalf("Expected 1 ready bead, got %d", len(readyBeads))
	}
}

func TestReadyAggregate(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")
	
	store, err := bd.OpenFromConfig(context.Background(), filepath.Join(tmpDir, "main"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	beadsFS := NewBeadsFS(store, context.Background())

	proj1 := filepath.Join(tmpDir, "project1")
	proj2 := filepath.Join(tmpDir, "project2")
	os.MkdirAll(proj1, 0755)
	os.MkdirAll(proj2, 0755)

	beadsFS.Mount("proj1", proj1)
	beadsFS.Mount("proj2", proj2)
	defer beadsFS.Umount("proj1")
	defer beadsFS.Umount("proj2")

	beadsFS.Write("proj1/ctl", []byte("init bd"), "test")
	beadsFS.Write("proj2/ctl", []byte("init bd"), "test")

	beadsFS.Write("proj1/ctl", []byte("new 'Task 1' 'Desc 1'"), "test")
	beadsFS.Write("proj2/ctl", []byte("new 'Task 2' 'Desc 2'"), "test")

	ready, err := beadsFS.Read("ready")
	if err != nil {
		t.Fatal(err)
	}

	var tasks []map[string]interface{}
	json.Unmarshal(ready, &tasks)

	proj1Count := 0
	proj2Count := 0
	for _, task := range tasks {
		mount, _ := task["mount"].(string)
		if mount == "proj1" {
			proj1Count++
		} else if mount == "proj2" {
			proj2Count++
		}
	}
	
	if proj1Count != 1 || proj2Count != 1 {
		t.Fatalf("Expected 1 task from each mount, got proj1=%d proj2=%d", proj1Count, proj2Count)
	}
}
