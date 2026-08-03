package home

import (
	"sync"
	"testing"
)

// One RoleManager serves every HTTP request, so its cache is read by one request
// while another is filling it. That is not a race Go merely reports — an
// unsynchronized map read against a concurrent write is
// `fatal error: concurrent map read and map write`, which no recover can catch.
// Two people loading the agents list at the same moment took the daemon down,
// and with it the supervision of every agent (#3565).
//
// Run with -race to see the diagnosis; without it, this still crashes the test
// binary outright on the unguarded map, which is the behavior that matters.
func TestTheRoleCacheSurvivesConcurrentUse(t *testing.T) {
	rm := newTestRoleManager(t)

	for _, name := range []string{"engineer", "reviewer", "trader", "manager"} {
		if err := rm.store.Save(&Role{
			Metadata: RoleMetadata{Name: name, MCPServers: []string{"mycel"}},
			Prompt:   "# " + name,
		}); err != nil {
			t.Fatal(err)
		}
	}

	const readers = 24
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := range readers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			name := []string{"engineer", "reviewer", "trader", "manager"}[i%4]
			for range 40 {
				if _, err := rm.ResolveRole(name); err != nil {
					t.Errorf("ResolveRole(%q): %v", name, err)
					return
				}
				if _, err := rm.LoadAllRoles(); err != nil {
					t.Errorf("LoadAllRoles: %v", err)
					return
				}
				rm.HasRole(name)
				rm.GetRole(name)
			}
		}(i)
	}

	close(start)
	wg.Wait()
}

// LoadAllRoles used to return the live cache, so a caller could still be reading
// the map it was handed while another request wrote to it — the same crash, just
// further from the scene.
func TestLoadAllRolesHandsBackACopy(t *testing.T) {
	rm := newTestRoleManager(t)
	if err := rm.store.Save(&Role{Metadata: RoleMetadata{Name: "engineer"}, Prompt: "# engineer"}); err != nil {
		t.Fatal(err)
	}

	all, err := rm.LoadAllRoles()
	if err != nil {
		t.Fatalf("LoadAllRoles: %v", err)
	}

	delete(all, "engineer")
	if _, ok := rm.cachedRole("engineer"); !ok {
		t.Error("mutating the returned map changed the manager's cache")
	}
}
