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
	Scope     string `json:"scope,omitempty" jsonschema:"'project' (default with project_id) or 'global'"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"project uuid (required for scope=project)"`
}

type docsListInput struct {
	docsScopeInput
	PathPrefix string `json:"path_prefix,omitempty" jsonschema:"path prefix filter"`
	Kind       string `json:"kind,omitempty" jsonschema:"'skill','context','note'"`
}

type docsReadInput struct {
	docsScopeInput
	Path     string `json:"path" jsonschema:"document logical path"`
	HeadOnly bool   `json:"head_only,omitempty" jsonschema:"return frontmatter + first 400 chars of body (with truncated flag) instead of the full document"`
}

type docsSearchInput struct {
	docsScopeInput
	Query string `json:"query" jsonschema:"plainto_tsquery"`
	Kind  string `json:"kind,omitempty" jsonschema:"kind filter"`
}

type docsWriteInput struct {
	docsScopeInput
	Path            string `json:"path" jsonschema:"logical path"`
	Content         string `json:"content" jsonschema:"full markdown body (frontmatter optional)"`
	Kind            string `json:"kind,omitempty" jsonschema:"override; otherwise from frontmatter or 'context'"`
	Message         string `json:"message,omitempty" jsonschema:"change message on the version row"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"must equal current_version (or 0 for new)"`
}

type docsDeleteInput struct {
	docsScopeInput
	Path            string `json:"path" jsonschema:"logical path"`
	Message         string `json:"message,omitempty" jsonschema:"change message"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"must equal current_version"`
}

type docsHistoryInput struct {
	docsScopeInput
	Path string `json:"path" jsonschema:"logical path"`
}

type docsReadVersionInput struct {
	docsScopeInput
	Path    string `json:"path"`
	Version int    `json:"version" jsonschema:"version number"`
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
		Description: "Reads a document: title, kind, body, frontmatter, current_version. head_only=true returns frontmatter + 400-char preview with {truncated, body_length} for catalogue checks.",
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
		if in.HeadOnly {
			const previewLimit = 400
			bodyLen := len(doc.ContentMD)
			truncated := bodyLen > previewLimit
			preview := doc.ContentMD
			if truncated {
				preview = doc.ContentMD[:previewLimit]
			}
			return jsonResult(map[string]any{
				"id":              doc.ID,
				"scope":           doc.Scope,
				"project_id":      doc.ProjectID,
				"path":            doc.Path,
				"kind":            doc.Kind,
				"title":           doc.Title,
				"description":     doc.Description,
				"frontmatter":     doc.Frontmatter,
				"current_version": doc.CurrentVersion,
				"updated_at":      doc.UpdatedAt,
				"content":         preview,
				"truncated":       truncated,
				"body_length":     bodyLen,
			})
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
		Description: "Creates/updates a document. Pass expected_version = current_version (0 for new). Conflict returns {error:'version_conflict', current_version}. Omitting expected_version is deprecated.",
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
		Description: "Soft-deletes a document (history preserved; rewriting resurrects). Pass expected_version = current_version. On conflict returns {error:'version_conflict', current_version}.",
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
		Description: "Lists every version of a document (metadata only). Use read_version for a body.",
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
