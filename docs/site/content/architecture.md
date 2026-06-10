---
title: Architecture diagram
section: Views
nav_order: 3
---

# Architecture diagram

The Architecture view is a textual model of the product, rendered
as a compound-layout SVG. Boxes are **nodes** (systems, services,
modules, components, externals). Arrows are **edges** with a kind
of their own (`calls`, `reads`, `writes`, `subscribes`, etc.). Both
nodes and edges can link a repository and a path so the diagram
maps onto the source tree.

![Architecture diagram with nested compound boxes and labelled edges](/screenshots/architecture-diagram.png)

## How it is maintained

Agents edit the graph via `nottario.arch.*` MCP tools
(`upsert_node`, `upsert_edge`, `move_node`, `remove_*`). The web
UI is read-only for the structure: humans browse it, focus a node,
follow edges to neighbours, and edit the right-rail description
inline. Anything structural — adding a service, reparenting a
module, adding an edge between two components — flows through an
agent.

The textual representation is intentional: it makes the graph
diffable, reviewable in PRs, and trivially editable by an LLM.

## What you see

- **Compound layout**: parents (systems, services) contain their
  children (modules, components). The renderer collapses dense
  subgraphs and expands them on click, so the canvas stays
  legible even when the project grows.
- **Right rail**: when a node is focused, the rail shows its
  description, the repo and path it links, and the inbound /
  outbound edges with their kinds and target slugs.
- **Search**: the global `/` shortcut filters nodes by slug or
  name. A matched node is highlighted; the rest dims.

## What it is good for

- Onboarding: a new contributor (human or agent) opens the
  Architecture page and sees the shape of the system before
  reading a single file.
- Refactor planning: "who depends on `backend.realtime`?" is one
  click on the rail's `← incoming` list.
- Drift detection: when the graph diverges from the code, the
  difference is visible and an agent can fix one or the other.
