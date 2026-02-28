package p9

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	
	bd "github.com/steveyegge/beads"
)

func TestBeadsFS_Mount(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := bd.OpenFromConfig(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()
	
	beadsFS := NewBeadsFS(store, context.Background())
	
	testCwd := filepath.Join(tmpDir, "project")
	os.MkdirAll(filepath.Join(testCwd, ".beads"), 0755)
	
	// Test mount
	err = beadsFS.Mount("test-project", testCwd)
	if err != nil {
		t.Fatalf("Mount failed: %v", err)
	}
	
	// Test duplicate mount
	err = beadsFS.Mount("test-project", testCwd)
	if err == nil {
		t.Fatal("Should have failed on duplicate mount")
	}
}

func TestBeadsFS_Umount(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := bd.OpenFromConfig(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()
	
	beadsFS := NewBeadsFS(store, context.Background())
	
	testCwd := filepath.Join(tmpDir, "project")
	os.MkdirAll(filepath.Join(testCwd, ".beads"), 0755)
	
	// Mount first
	err = beadsFS.Mount("test-project", testCwd)
	if err != nil {
		t.Fatalf("Mount failed: %v", err)
	}
	
	// Test umount
	err = beadsFS.Umount("test-project")
	if err != nil {
		t.Fatalf("Umount failed: %v", err)
	}
	
	// Test umount non-existent
	err = beadsFS.Umount("nonexistent")
	if err == nil {
		t.Fatal("Should have failed on non-existent mount")
	}
}

func TestBeadsFS_MountRouting(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := bd.OpenFromConfig(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()
	
	beadsFS := NewBeadsFS(store, context.Background())
	
	testCwd := filepath.Join(tmpDir, "project")
	os.MkdirAll(filepath.Join(testCwd, ".beads"), 0755)
	
	// Mount
	err = beadsFS.Mount("test-project", testCwd)
	if err != nil {
		t.Fatalf("Mount failed: %v", err)
	}
	defer beadsFS.Umount("test-project")
	
	// Test cwd endpoint
	data, err := beadsFS.Read("agent/beads/test-project/cwd")
	if err != nil {
		t.Fatalf("Read cwd failed: %v", err)
	}
	if string(data) != testCwd {
		t.Fatalf("Expected cwd %s, got %s", testCwd, string(data))
	}
	
	// Test non-existent mount
	_, err = beadsFS.Read("agent/beads/nonexistent/cwd")
	if err == nil {
		t.Fatal("Should have failed on non-existent mount")
	}
}
