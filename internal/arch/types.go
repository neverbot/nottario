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
	ID            uuid.UUID
	ProjectID     uuid.UUID
	Slug          string
	ParentID      *uuid.UUID
	Kind          string
	Name          string
	DescriptionMD string
	Metadata      map[string]any
	LinkedRepo    *string
	LinkedPath    *string
	Position      int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Edge is a directed relationship between two nodes (possibly across
// different levels of the tree).
type Edge struct {
	ID            uuid.UUID
	ProjectID     uuid.UUID
	FromNodeID    uuid.UUID
	ToNodeID      uuid.UUID
	Kind          string
	Label         string
	DescriptionMD string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Kind labels a node type. Projects start with a seeded default
// catalogue (system, service, module, component, external) and may
// add custom kinds.
type Kind struct {
	ProjectID   uuid.UUID
	Key         string
	Label       string
	Icon        string
	Color       string
	Description string
	IsDefault   bool
	CreatedAt   time.Time
}

// NodeLink attaches a markdown document (by path) or a task (by
// uuid) to a node.
type NodeLink struct {
	ProjectID uuid.UUID
	NodeID    uuid.UUID
	LinkType  string // "doc" or "task"
	TargetID  string
	CreatedAt time.Time
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
