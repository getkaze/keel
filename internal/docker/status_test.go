package docker

import (
	"testing"

	"github.com/getkaze/keel/internal/model"
)

func TestContainerToStatus_Nil(t *testing.T) {
	if got := ContainerToStatus(nil); got != model.StatusMissing {
		t.Errorf("expected StatusMissing for nil, got %q", got)
	}
}

func TestContainerToStatus_Running(t *testing.T) {
	ci := &ContainerInfo{State: "running", Status: "Up 2 hours"}
	if got := ContainerToStatus(ci); got != model.StatusRunning {
		t.Errorf("expected StatusRunning, got %q", got)
	}
}

func TestContainerToStatus_Unhealthy(t *testing.T) {
	ci := &ContainerInfo{State: "running", Status: "Up 2 hours (unhealthy)"}
	if got := ContainerToStatus(ci); got != model.StatusUnhealthy {
		t.Errorf("expected StatusUnhealthy, got %q", got)
	}
}

func TestContainerToStatus_Stopped(t *testing.T) {
	ci := &ContainerInfo{State: "exited", Status: "Exited (1) 5 minutes ago"}
	if got := ContainerToStatus(ci); got != model.StatusStopped {
		t.Errorf("expected StatusStopped, got %q", got)
	}
}

func TestContainerToStatus_CaseInsensitive(t *testing.T) {
	ci := &ContainerInfo{State: "RUNNING", Status: "Up 1 hour"}
	if got := ContainerToStatus(ci); got != model.StatusRunning {
		t.Errorf("expected StatusRunning for uppercase state, got %q", got)
	}
}

func TestMatchServiceToContainer_KeelPrefix(t *testing.T) {
	containers := []ContainerInfo{
		{Names: "/keel-mysql", State: "running"},
		{Names: "/keel-redis", State: "running"},
	}
	ci := MatchServiceToContainer("mysql", "", containers)
	if ci == nil {
		t.Fatal("expected match for 'mysql' via keel- prefix, got nil")
	}
	if ci.Names != "/keel-mysql" {
		t.Errorf("expected /keel-mysql, got %q", ci.Names)
	}
}

func TestMatchServiceToContainer_DirectName(t *testing.T) {
	containers := []ContainerInfo{
		{Names: "/mysql", State: "running"},
	}
	ci := MatchServiceToContainer("mysql", "", containers)
	if ci == nil {
		t.Fatal("expected match for direct name 'mysql', got nil")
	}
}

func TestMatchServiceToContainer_NotFound(t *testing.T) {
	containers := []ContainerInfo{
		{Names: "/keel-redis", State: "running"},
	}
	ci := MatchServiceToContainer("mysql", "", containers)
	if ci != nil {
		t.Errorf("expected nil for no match, got %+v", ci)
	}
}

func TestMatchServiceToContainer_EmptyList(t *testing.T) {
	ci := MatchServiceToContainer("mysql", "", nil)
	if ci != nil {
		t.Errorf("expected nil for empty container list, got %+v", ci)
	}
}

func TestMatchServiceToContainer_ExplicitHostname(t *testing.T) {
	containers := []ContainerInfo{
		{Names: "myapp-mysql57", State: "running"},
	}
	ci := MatchServiceToContainer("mysql57", "myapp-mysql57", containers)
	if ci == nil {
		t.Fatal("expected match via explicit hostname 'myapp-mysql57', got nil")
	}
	if ci.Names != "myapp-mysql57" {
		t.Errorf("expected myapp-mysql57, got %q", ci.Names)
	}
}

func TestMergeContainers_Deduplicates(t *testing.T) {
	a := []ContainerInfo{
		{ID: "aaa", Names: "/keel-mysql"},
		{ID: "bbb", Names: "/keel-redis"},
	}
	b := []ContainerInfo{
		{ID: "bbb", Names: "/keel-redis"}, // duplicate
		{ID: "ccc", Names: "/keel-mongo"},
	}
	merged := mergeContainers(a, b)
	if len(merged) != 3 {
		t.Fatalf("expected 3 unique containers, got %d", len(merged))
	}
	ids := map[string]bool{}
	for _, c := range merged {
		ids[c.ID] = true
	}
	for _, id := range []string{"aaa", "bbb", "ccc"} {
		if !ids[id] {
			t.Errorf("missing container %s in merged result", id)
		}
	}
}

func TestMergeContainers_EmptyLists(t *testing.T) {
	merged := mergeContainers(nil, nil)
	if len(merged) != 0 {
		t.Errorf("expected 0 containers for empty lists, got %d", len(merged))
	}
}

func TestMergeContainers_OneEmpty(t *testing.T) {
	a := []ContainerInfo{{ID: "aaa", Names: "/keel-mysql"}}
	merged := mergeContainers(a, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 container, got %d", len(merged))
	}
	if merged[0].ID != "aaa" {
		t.Errorf("expected id aaa, got %q", merged[0].ID)
	}
}
