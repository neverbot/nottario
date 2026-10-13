package mcp

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

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
	HeadOnly bool   `json:"head_only,omitempty" jsonschema:"return parsed frontmatter + the first 400 chars of the document (with truncated flag) instead of the whole document"`
}

type docsSearchInput struct {
	docsScopeInput
	Query string `json:"query" jsonschema:"plainto_tsquery"`
	Kind  string `json:"kind,omitempty" jsonschema:"kind filter"`
}

type docsWriteInput struct {
	docsScopeInput
	Path            string `json:"path" jsonschema:"logical path"`
	Content         string `json:"content" jsonschema:"the complete markdown document, frontmatter included; stored byte for byte"`
	Kind            string `json:"kind,omitempty" jsonschema:"override; otherwise from frontmatter or 'context'"`
	Message         string `json:"message,omitempty" jsonschema:"change message on the version row"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"must equal current_version (or 0 for new)"`
}

type docsStatInput struct {
	docsScopeInput
	Path string `json:"path" jsonschema:"logical path"`
}

type docsAppendInput struct {
	docsScopeInput
	Path            string `json:"path" jsonschema:"logical path"`
	Content         string `json:"content" jsonschema:"markdown to add at the end of the body"`
	Message         string `json:"message,omitempty" jsonschema:"change message on the version row"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"must equal current_version"`
}

type docsUploadURLInput struct {
	ProjectID       string `json:"project_id" jsonschema:"project uuid"`
	Path            string `json:"path" jsonschema:"logical path of the document to create or replace"`
	ExpectedVersion *int   `json:"expected_version" jsonschema:"current_version from docs.stat, or 0 to create"`
	FileSHA256      string `json:"file_sha256" jsonschema:"hex SHA-256 of the exact bytes you will upload: the whole file, frontmatter included"`
	Kind            string `json:"kind,omitempty" jsonschema:"override; otherwise from frontmatter or 'context'"`
	Message         string `json:"message,omitempty" jsonschema:"change message on the version row"`
}

type docsDownloadURLInput struct {
	ProjectID string `json:"project_id" jsonschema:"project uuid"`
	Path      string `json:"path" jsonschema:"logical path of the document to download"`
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
		Description: "Only for clients without a shell or HTTP access to the server; otherwise use docs.download_url. Returns the whole document: content (frontmatter included), parsed frontmatter, current_version. head_only=true returns the frontmatter and the first 400 chars.",
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
		Description: "Creates or replaces a document from text you are composing; for a file on disk use docs.upload_url. Pass expected_version = current_version (0 to create). Ack {path, current_version, updated_at}; a conflict returns {error:'version_conflict', current_version}.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsWriteInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		if scope == docs.ScopeGlobal {
			if err := identity.RequireGlobalWrite(c); err != nil {
				return toolError(err.Error())
			}
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
		// Slim ack. Echoing the document back would hand the caller a
		// second copy of the body it just composed — on a large
		// document that is the single most expensive thing this tool
		// could do to an agent's context.
		//
		// current_version is the one field worth returning: without it
		// the caller has to docs.read before its next write just to
		// learn the number, which costs far more than it saves.
		return jsonResult(map[string]any{
			"path":            doc.Path,
			"current_version": doc.CurrentVersion,
			"updated_at":      doc.UpdatedAt,
		})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.stat",
		Description: "Fingerprints a document without its body: {path, current_version, size_bytes, content_sha256, updated_at}. content_sha256 equals sha256sum of the whole file, frontmatter included. Use it to decide whether to download or write.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsStatInput) (*sdk.CallToolResult, any, error) {
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		st, err := docs.ReadStat(ctx, d.Pool, scope, pid, in.Path)
		if errors.Is(err, docs.ErrNotFound) {
			return toolError("document not found")
		}
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(st)
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.append",
		Description: "Adds markdown to the end of an existing document without resending it. Pass expected_version = current_version. Same ack as docs.write; never creates.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsAppendInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		scope, pid, err := resolveDocScope(ctx, d, in.docsScopeInput)
		if err != nil {
			return toolError(err.Error())
		}
		if scope == docs.ScopeGlobal {
			if err := identity.RequireGlobalWrite(c); err != nil {
				return toolError(err.Error())
			}
		}
		if in.ExpectedVersion == nil {
			log.Printf("mcp docs.append: deprecated call without expected_version (user=%s path=%q scope=%s)", c.UserID, in.Path, scope)
		}
		doc, err := docs.Append(ctx, d.Pool, docs.AppendParams{
			Scope: scope, ProjectID: pid, Path: in.Path,
			Content:         in.Content,
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
		if errors.Is(err, docs.ErrNotFound) {
			return toolError("document not found (append never creates; use docs.write for the first version)")
		}
		if err != nil {
			return toolError(err.Error())
		}
		return jsonResult(map[string]any{
			"path":            doc.Path,
			"current_version": doc.CurrentVersion,
			"updated_at":      doc.UpdatedAt,
		})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name: "nottario.docs.upload_url",
		// The how-to lives here, in the tool description, which the
		// client already holds for the whole session. Repeating it in
		// every response charged the same ~150 tokens per upload.
		Description: "Signs a single-use URL to create or replace a project document from a file on disk. file_sha256 = SHA-256 of the exact file; expected_version = current_version (docs.stat or download_url), 0 to create. Returns {upload_url, expires_at, expires_in_seconds}. Then curl -fsS -X PUT --data-binary @<file> '<upload_url>' (quote it; no Authorization header). Valid 5 minutes, once, on that version: 409 someone wrote first, 403 expired, 400 bytes did not match.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsUploadURLInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if len(d.SessionKey) == 0 {
			// Never issue an unsigned upload URL.
			return toolError(docs.ErrUploadNoKey.Error())
		}
		if c.Source != identity.SourceToken || c.TokenID == uuid.Nil {
			return toolError("upload URLs are only issued to API-token callers")
		}
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		if strings.TrimSpace(in.Path) == "" {
			return toolError("path is required")
		}
		if in.ExpectedVersion == nil || *in.ExpectedVersion < 0 {
			return toolError("expected_version is required: current_version from docs.stat, or 0 to create")
		}
		sha := strings.ToLower(strings.TrimSpace(in.FileSHA256))
		if !docs.ValidSHA256Hex(sha) {
			return toolError("file_sha256 must be the 64-character hex SHA-256 of the exact bytes you will upload")
		}
		// Fail fast on a stale version instead of letting the agent
		// upload bytes that are bound to be refused. The upload still
		// re-checks atomically; this only saves a wasted transfer.
		if st, err := docs.ReadStat(ctx, d.Pool, docs.ScopeProject, &pid, in.Path); err == nil && st.CurrentVersion != *in.ExpectedVersion {
			return jsonResult(map[string]any{
				"error":           "version_conflict",
				"current_version": st.CurrentVersion,
				"message":         "the document is not at expected_version; run docs.stat and decide again",
			})
		}
		exp := time.Now().Add(docs.UploadTTL).Unix()
		q, err := docs.SignUpload(d.SessionKey, docs.UploadGrant{
			ProjectID:       pid,
			Path:            in.Path,
			ExpectedVersion: *in.ExpectedVersion,
			FileSHA256:      sha,
			Kind:            in.Kind,
			Message:         in.Message,
			UserID:          c.UserID,
			TokenID:         c.TokenID,
			ExpiresAt:       exp,
		})
		if err != nil {
			return toolError(err.Error())
		}
		base := strings.TrimRight(externalBaseURL(ctx), "/")
		return jsonResult(map[string]any{
			"upload_url":         base + "/api/docs/upload?" + q.Encode(),
			"expires_at":         time.Unix(exp, 0).UTC().Format(time.RFC3339),
			"expires_in_seconds": int(docs.UploadTTL / time.Second),
		})
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "nottario.docs.download_url",
		Description: "Signs a URL to fetch a project document's current version to a file: how to read or edit documents when you have a shell. Returns {download_url, version, expires_in_seconds}. Then curl -fsS -o <file> '<download_url>' (quote it; no Authorization header). Valid 5 minutes. Write it back with docs.upload_url, expected_version = version.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in docsDownloadURLInput) (*sdk.CallToolResult, any, error) {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		if len(d.SessionKey) == 0 {
			return toolError(docs.ErrUploadNoKey.Error())
		}
		if c.Source != identity.SourceToken || c.TokenID == uuid.Nil {
			return toolError("download URLs are only issued to API-token callers")
		}
		pid, err := uuid.Parse(in.ProjectID)
		if err != nil {
			return toolError("project_id must be a uuid")
		}
		if err := requireProjectAccess(ctx, d, pid); err != nil {
			return toolError(err.Error())
		}
		st, err := docs.ReadStat(ctx, d.Pool, docs.ScopeProject, &pid, in.Path)
		if errors.Is(err, docs.ErrNotFound) {
			return toolError("document not found: " + in.Path)
		}
		if err != nil {
			return toolError(err.Error())
		}
		exp := time.Now().Add(docs.DownloadTTL).Unix()
		q, err := docs.SignDownload(d.SessionKey, docs.DownloadGrant{
			ProjectID: pid,
			Path:      st.Path,
			Version:   st.CurrentVersion,
			UserID:    c.UserID,
			TokenID:   c.TokenID,
			ExpiresAt: exp,
		})
		if err != nil {
			return toolError(err.Error())
		}
		base := strings.TrimRight(externalBaseURL(ctx), "/")
		return jsonResult(map[string]any{
			"download_url":       base + "/api/docs/download?" + q.Encode(),
			"version":            st.CurrentVersion,
			"expires_in_seconds": int(docs.DownloadTTL / time.Second),
		})
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
		if scope == docs.ScopeGlobal {
			if err := identity.RequireGlobalWrite(c); err != nil {
				return toolError(err.Error())
			}
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
