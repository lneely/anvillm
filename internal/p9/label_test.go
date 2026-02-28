package p9

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	
	bd "github.com/steveyegge/beads"
)

func TestLabelClaimable(t *testing.T) {
	tmpDir := t.TempDir()
	
	store, err := bd.OpenFromConfig(context.Background(), tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	
	beadsFS := NewBeadsFS(store, context.Background())
	
	// Initialize
	err = beadsFS.Write("ctl", []byte("init bd"), "test")
	if err != nil {
		t.Fatal(err)
	}
	
	// Create a bead
	err = beadsFS.Write("ctl", []byte("new 'Test Task' 'Test description'"), "test")
	if err != nil {
		t.Fatal(err)
	}
	
	// List beads to get ID
	list, err := beadsFS.Read("list")
	if err != nil {
		t.Fatal(err)
	}
	
	var beads []map[string]interface{}
	json.Unmarshal(list, &beads)
	if len(beads) == 0 {
		t.Fatal("no beads created")
	}
	
	beadID := beads[0]["id"].(string)
	t.Logf("Created bead: %s", beadID)
	
	// Label it as claimable
	cmd := fmt.Sprintf("label %s claimable", beadID)
	t.Logf("Running command: %s", cmd)
	err = beadsFS.Write("ctl", []byte(cmd), "test")
	if err != nil {
		t.Fatalf("label failed: %v", err)
	}
	t.Logf("Label command succeeded")
	
	// Read it back
	list, err = beadsFS.Read("list")
	if err != nil {
		t.Fatal(err)
	}
	
	json.Unmarshal(list, &beads)
	t.Logf("Full bead data: %+v", beads[0])
	labels, ok := beads[0]["labels"].([]interface{})
	if !ok {
		labels = []interface{}{}
	}
	t.Logf("Labels: %v", labels)
	
	hasClaimable := false
	for _, l := range labels {
		if l.(string) == "claimable" {
			hasClaimable = true
		}
	}
	
	if !hasClaimable {
		t.Fatal("claimable label not found")
	}
}
