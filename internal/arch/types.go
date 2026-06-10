// Package arch owns the architectural diagram of a project: a tree
// of nodes (services, modules, components, ...) plus directed edges
// describing how they relate. Nodes carry a project-scoped readable
// slug so agents can refer to them by name in markdown, commits and
// MCP calls. The internal primary key remains a uuid for FK joins.
//
// In v1 the diagram is built and maintained by agents through MCP
// calls (the skill teaches the patterns). Humans read it through the
// web UI but cannot edit — they ask agents instead. A future
// milestone (M5) layers the graph visualisation on top of the same
// data model.
package arch

import (
	"time"

	"github.com/google/uuid"
)

// Node is a single architecture box: a system, service, module,
// component, external dependency, or any custom kind defined per
// project.
type Node struct {
	ID            uuid.UUID      `json:"id"`
	ProjectID     uuid.UUID      `json:"project_id"`
	Slug          string         `json:"slug"`
	ParentID      *uuid.UUID     `json:"parent_id"`
	Kind          string         `json:"kind"`
	Name          string         `json:"name"`
	DescriptionMD string         `json:"description"`
	Metadata      map[string]any `json:"metadata"`
	LinkedRepo    *string        `json:"linked_repo"`
	LinkedPath    *string        `json:"linked_path"`
	Position      int            `json:"position"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// Edge is a directed relationship between two nodes (possibly across
// different levels of the tree).
type Edge struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	FromNodeID    uuid.UUID `json:"from_node_id"`
	ToNodeID      uuid.UUID `json:"to_node_id"`
	Kind          string    `json:"kind"`
	Label         string    `json:"label"`
	DescriptionMD string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Kind labels a node type. Projects start with a seeded default
// catalogue (system, service, module, component, external) and may
// add custom kinds.
type Kind struct {
	ProjectID   uuid.UUID `json:"project_id"`
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Icon        string    `json:"icon"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}

// NodeLink attaches a markdown document (by path) or a task (by
// uuid) to a node.
type NodeLink struct {
	ProjectID uuid.UUID `json:"project_id"`
	NodeID    uuid.UUID `json:"node_id"`
	LinkType  string    `json:"link_type"` // "doc" or "task"
	TargetID  string    `json:"target_id"`
	CreatedAt time.Time `json:"created_at"`
}

// DefaultKinds is the seed catalogue inserted into a project the
// first time architecture is touched. Custom kinds added later live
// alongside these.
var DefaultKinds = []Kind{
	{Key: "system", Label: "System", Icon: "package", Color: "#1f2328",
		Description: "Top-level container — the product as a whole."},
	{Key: "service", Label: "Service", Icon: "server", Color: "#1f6feb",
		Description: "A microservice, a long-running process, or a single binary."},
	{Key: "module", Label: "Module", Icon: "stack", Color: "#2da44e",
		Description: "A logical grouping inside a service or system."},
	{Key: "component", Label: "Component", Icon: "puzzle", Color: "#bf8700",
		Description: "A concrete piece: controller, service, model, repository."},
	{Key: "external", Label: "External", Icon: "globe", Color: "#a371f7",
		Description: "An external system (GitHub, Postgres, third-party API)."},
}
