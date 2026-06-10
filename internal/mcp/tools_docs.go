package mcp

import (
	"context"
	"errors"
	"log"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
)

// Common shape for docs tool inputs. Scope defaults to 'project' when
// project_id is present, or 'global' when it is empty.
type docsScopeInput struct {
	Scope     string `json:"scope,omitempty" jsonschema:"'project' (default if project_id is set) or 'global'"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"uuid of the project (required when scope='project')"`
}

type docsListInput struct {
	docsScopeInput
	PathPrefix string `json:"path_prefix,omitempty" jsonschema:"optional path prefix to narrow the listing"`
	Kind       string `json:"kind,omitempty" jsonschema:"optional kind filter: 'skill', 'context' or 'note'"`
}

type docsReadInput struct {
	docsScopeInput
	Path string `json:"path" jsonschema:"logical path of the document (e.g. 'projects/abc/context/glossary.md')"`
}

type docsSearchInput struct {
	docsScopeInput
	Query string `json:"query" jsonschema:"full-text query (plainto_tsquery semantics)"`
	Kind  string `json:"kind,omitempty" jsonschema:"optional kind filter"`
}

type docsWriteInput struct {
	docsScopeInput
	Path            string `json:"path" jsonschema:"logical path"`
	Content         string `json:"content" jsonschema:"full markdown body including optional YAML frontmatter at the top"`
	Kind            string `json:"kind,omitempty" jsonschema:"optional override; otherwise read from frontmatter or defaults to 'context'"`
	Message         string `json:"message,omitempty" jsonschema:"short explanation of the change, stored on the version row"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"optional optimistic-concurrency check: must equal current_version (or 0 for new docs)"`
}

type docsDeleteInput struct {
	docsScopeInput
	Path            string `json:"path" jsonschema:"logical path"`
	Message         string `json:"message,omitempty" jsonschema:"short explanation, stored on the version row"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"optional optimistic-concurrency check: must equal current_version"`
}

type docsHistoryInput struct {
	docsScopeInput
	Path string `json:"path" jsonschema:"logical path"`
}

type docsReadVersionInput struct {
	docsScopeInput
	Path    string `json:"path"`
	Version int    `json:"version" jsonschema:"version number to fetch"`
}

func registerDocs(server *sdk.Server, d Deps) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.list",
		Description: "Lists documents in a scope. Returns lightweight summaries (no body). Filter by path prefix and kind.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsListInput) (*sdk.CallToolResult, any, error) {
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		list, err := docs.List(ctx, d.Pool, docs.ListFilter{
			Scope: scope, ProjectID: pid,
			PathPrefix: in.PathPrefix,
			Kind:       docs.Kind(in.Kind),
		})
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"documents": list})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.read",
		Description: "Reads a single document by path. Returns title, kind, body markdown, frontmatter and current_version.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsReadInput) (*sdk.CallToolResult, any, error) {
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		doc, err := docs.Read(ctx, d.Pool, scope, pid, in.Path)
		if errors.Is(err, docs.ErrNotFound) {
			return toolError("document not found")
		}
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(doc)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.search",
		Description: "Full-text search over documents in a scope. Returns hits ranked by relevance.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsSearchInput) (*sdk.CallToolResult, any, error) {
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		hits, err := docs.Search(ctx, d.Pool, in.Query, docs.SearchFilter{
			Scope: scope, ProjectID: pid, Kind: docs.Kind(in.Kind),
		})
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"hits": hits})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.write",
		Description: "Creates or updates a document. Pass the full markdown including optional YAML frontmatter. Always pass expected_version: equal to the current_version returned by the most recent docs.read, or 0 when creating a new document. Omitting expected_version is DEPRECATED and logs a server-side warning — it skips the optimistic-concurrency check and risks silently overwriting concurrent edits.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsWriteInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		if scope == docs.ScopeGlobal && !c.IsAdmin {
			return toolError("only admins can modify global documents")
		}
		if in.ExpectedVersion == nil {
			log.Printf("mcp docs.write: deprecated call without expected_version (user=%s path=%q scope=%s)", c.UserID, in.Path, scope)
		}
		doc, err := docs.Write(ctx, d.Pool, docs.WriteParams{
			Scope: scope, ProjectID: pid,
			Path: in.Path, Kind: docs.Kind(in.Kind),
			ContentMD:       in.Content,
			Message:         in.Message,
			ExpectedVersion: in.ExpectedVersion,
		}, docs.Authorship{UserID: ptrUUID(c.UserID), TokenID: ptrUUID(c.TokenID)})
		var vc *docs.VersionConflictError
		if errors.As(err, &vc) {
			return jsonResult(map[string]any{
				"error":           "version_conflict",
				"current_version": vc.CurrentVersion,
				"message":         "re-read the document and retry with the latest current_version",
			})
		}
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(doc)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.delete",
		Description: "Soft-deletes a document. The history is preserved; rewriting the same path resurrects it. Pass expected_version equal to the current_version returned by the most recent docs.read. Omitting expected_version is DEPRECATED and logs a server-side warning.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsDeleteInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		if scope == docs.ScopeGlobal && !c.IsAdmin {
			return toolError("only admins can modify global documents")
		}
		if in.ExpectedVersion == nil {
			log.Printf("mcp docs.delete: deprecated call without expected_version (user=%s path=%q scope=%s)", c.UserID, in.Path, scope)
		}
		err = docs.DeleteWithParams(ctx, d.Pool, docs.DeleteParams{
			Scope: scope, ProjectID: pid, Path: in.Path, Message: in.Message,
			ExpectedVersion: in.ExpectedVersion,
		}, docs.Authorship{UserID: ptrUUID(c.UserID), TokenID: ptrUUID(c.TokenID)})
		var vc *docs.VersionConflictError
		if errors.As(err, &vc) {
			return jsonResult(map[string]any{
				"error":           "version_conflict",
				"current_version": vc.CurrentVersion,
				"message":         "re-read the document and retry with the latest current_version",
			})
		}
		if errors.Is(err, docs.ErrNotFound) {
			return toolError("document not found")
		}
		if err != nil {
			return toolError(err.Error())
		}
		return textResult("ok")
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.history",
		Description: "Lists every version of a document (metadata only, no bodies). Use read_version to fetch a specific version body.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsHistoryInput) (*sdk.CallToolResult, any, error) {
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		doc, err := docs.Read(ctx, d.Pool, scope, pid, in.Path)
		if errors.Is(err, docs.ErrNotFound) {
			return toolError("document not found")
		}
		if err != nil {
			return toolError(err.Error())
		}
		versions, err := docs.History(ctx, d.Pool, doc.ID)
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{"versions": versions})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.read_version",
		Description: "Reads a specific historical version of a document.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsReadVersionInput) (*sdk.CallToolResult, any, error) {
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		doc, err := docs.Read(ctx, d.Pool, scope, pid, in.Path)
		if errors.Is(err, docs.ErrNotFound) {
			return toolError("document not found")
		}
		if err != nil {
			return toolError(err.Error())
		}
		v, err := docs.ReadVersion(ctx, d.Pool, doc.ID, in.Version)
		if errors.Is(err, docs.ErrNotFound) {
			return toolError("version not found")
		}
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(v)
	})
}

// resolveDocScope canonicalises (scope, project_id) the same way the
// REST handler does, applying project-membership rules for non-admins.
func resolveDocScope(ctx context.Context, d Deps, in docsScopeInput) (docs.Scope, *uuid.UUID, error) {
	c, err := callerFromContext(ctx)
	if err != nil {
		return "", nil, err
	}
	scope := docs.Scope(in.Scope)
	if scope == "" {
		if in.ProjectID == "" {
			scope = docs.ScopeGlobal
		} else {
			scope = docs.ScopeProject
		}
	}
	if !docs.ValidScope(scope) {
		return "", nil, errors.New("invalid scope")
	}
	if scope == docs.ScopeGlobal {
		return scope, nil, nil
	}
	if in.ProjectID == "" {
		return "", nil, errors.New("project_id is required when scope=project")
	}
	pid, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return "", nil, errors.New("project_id must be a uuid")
	}
	if err := identity.RequireProjectScope(c, pid); err != nil {
		return "", nil, err
	}
	if !c.IsAdmin {
		roles, err := identity.UserRoleIDs(ctx, d.Pool, c.UserID, pid)
		if err != nil {
			return "", nil, err
		}
		if len(roles) == 0 {
			return "", nil, errors.New("not a project member")
		}
	}
	return scope, &pid, nil
}

// ptrUUID returns a pointer to a uuid value, or nil if the value is
// the zero uuid.
func ptrUUID(u uuid.UUID) *uuid.UUID {
	if u == uuid.Nil {
		return nil
	}
	return &u
}
