package web

import "testing"

// Exercises the `nottario.projects.*` MCP tool family through the
// real streamable-HTTP transport. The fixture seeds one user + one
// project; tests call list / get / list_priorities / list_roles /
// reorder_roles via the MCP client and assert the response shape.
func TestMCP_Projects_ListAndGet(t *testing.T) {
	f := newMCPFixture(t, 13310, "proj-tester")

	// list returns at least our seeded project.
	var listOut struct {
		Projects []map[string]any `json:"projects"`
	}
	f.callJSON(t, "nottario.projects.list", map[string]any{}, &listOut)
	if len(listOut.Projects) == 0 {
		t.Fatal("projects.list returned no rows")
	}
	found := false
	for _, p := range listOut.Projects {
		if p["id"] == f.projectID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("seeded project %s not in list: %+v", f.projectID, listOut.Projects)
	}

	// get returns the same project.
	var getOut map[string]any
	f.callJSON(t, "nottario.projects.get", map[string]any{
		"project_id": f.projectID,
	}, &getOut)
	if getOut["id"] != f.projectID {
		t.Errorf("projects.get ID mismatch: %+v", getOut)
	}
}

func TestMCP_Projects_PrioritiesAndRoles(t *testing.T) {
	f := newMCPFixture(t, 13311, "prio-tester")

	var pri struct {
		Priorities []map[string]any `json:"priorities"`
	}
	f.callJSON(t, "nottario.projects.list_priorities", map[string]any{
		"project_id": f.projectID,
	}, &pri)
	keys := map[string]bool{}
	for _, p := range pri.Priorities {
		keys[p["key"].(string)] = true
	}
	for _, want := range []string{"low", "medium", "high", "critical"} {
		if !keys[want] {
			t.Errorf("default priority %q missing: %+v", want, pri.Priorities)
		}
	}

	var roles struct {
		Roles []map[string]any `json:"roles"`
	}
	f.callJSON(t, "nottario.projects.list_roles", map[string]any{
		"project_id": f.projectID,
	}, &roles)
	if len(roles.Roles) == 0 {
		t.Fatal("list_roles returned 0 rows")
	}
	// Reorder: take the role IDs in reverse order and feed them back.
	ids := make([]any, 0, len(roles.Roles))
	for i := len(roles.Roles) - 1; i >= 0; i-- {
		ids = append(ids, roles.Roles[i]["id"])
	}
	f.callJSON(t, "nottario.projects.reorder_roles", map[string]any{
		"project_id": f.projectID,
		"role_ids":   ids,
	}, nil)

	// list_roles again — the first one should now be what used to be last.
	var rolesAfter struct {
		Roles []map[string]any `json:"roles"`
	}
	f.callJSON(t, "nottario.projects.list_roles", map[string]any{
		"project_id": f.projectID,
	}, &rolesAfter)
	if rolesAfter.Roles[0]["id"] != ids[0] {
		t.Errorf("reorder did not stick: first role %v, expected %v", rolesAfter.Roles[0]["id"], ids[0])
	}
}

func TestMCP_Projects_GetMissingErrors(t *testing.T) {
	f := newMCPFixture(t, 13312, "missing")
	msg := f.callExpectErr(t, "nottario.projects.get", map[string]any{
		"project_id": "00000000-0000-0000-0000-000000000000",
	})
	if msg == "" {
		t.Error("expected non-empty error text")
	}
}
