package avatar

import (
	"errors"
	"io"
	stdhttp "net/http"
	"net/url"
	"strconv"
	"strings"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
)

// The handlers are thin on purpose: read the input, ask the service, answer. No
// rule, database handle or Model construction lives here. A handler that reached
// data directly would skip the service's policy boundary, and the layout makes
// that visible rather than relying on review.
//
// Who is asking comes from ctx.User, which reads the subject RequireAuth put on
// the request. No handler loads the session again or takes an identity from the
// input, and none takes a tenant from anywhere: the tenant is the Grant's.
//
// An error from the service is returned as it is, and the router answers it:
// validation.Errors goes back where the request came from with the messages in
// the flash, ErrNotFound is 404 because it wraps model.ErrModelNotFound, and a
// policy refusal is 403. The answer is the status and its standard sentence,
// never the error's own text. The one exception is an htmx request: it cannot
// follow a redirect into a fragment, so a refused change answers with the editor
// again, with the messages in it.

// handler is the handler of one route of the table.
func (m *Module) handler(rt route) func(*fhttp.Context) error {
	kind, team := rt.kind, rt.team
	switch {
	case rt.action == "show" && !team:
		return func(ctx *fhttp.Context) error { return m.show(ctx, kind) }
	case rt.action == "show":
		return func(ctx *fhttp.Context) error { return m.teamShow(ctx, kind) }
	case rt.action == "remove" && team:
		return func(ctx *fhttp.Context) error { return m.teamRemove(ctx, kind) }
	case rt.action == "editor":
		return func(ctx *fhttp.Context) error { return m.editor(ctx, kind) }
	case rt.action == "upload":
		return func(ctx *fhttp.Context) error { return m.upload(ctx, kind) }
	case rt.action == "gravatar":
		return func(ctx *fhttp.Context) error { return m.gravatar(ctx, kind) }
	case rt.action == "style":
		return func(ctx *fhttp.Context) error { return m.style(ctx, kind) }
	default:
		return func(ctx *fhttp.Context) error { return m.remove(ctx, kind) }
	}
}

// show answers the picture an address names.
func (m *Module) show(ctx *fhttp.Context, kind Kind) error {
	who, _ := ctx.User()
	size, err := strconv.Atoi(ctx.Param("size"))
	if err != nil {
		return ErrNotFound
	}
	opened, err := m.avatars.Open(ctx.Ctx(), who, Ref{Kind: kind, ID: ctx.Param("id")}, ctx.Param("version"), size, ctx.Header("If-None-Match"))
	if err != nil {
		return err
	}
	return m.serve(ctx, opened)
}

// teamShow answers the picture an address names, to the platform team.
func (m *Module) teamShow(ctx *fhttp.Context, kind Kind) error {
	who, _ := ctx.User()
	size, err := strconv.Atoi(ctx.Param("size"))
	if err != nil {
		return ErrNotFound
	}
	opened, err := m.team.Open(ctx.Ctx(), who, Ref{Kind: kind, ID: ctx.Param("id")}, ctx.Query("space"), ctx.Param("version"), size, ctx.Header("If-None-Match"))
	if err != nil {
		return err
	}
	return m.serve(ctx, opened)
}

// serve writes a picture: the type the package detected and stored, never one
// the sender announced, never sniffed, and with the entity tag and the lifetime
// that let a browser and a CDN keep it. The address holds the version, so the
// lifetime is as long as it may be.
func (m *Module) serve(ctx *fhttp.Context, opened Opened) error {
	header := ctx.Response.Header()
	if opened.Redirect != "" {
		header.Set("Cache-Control", "private, max-age=60")
		stdhttp.Redirect(ctx.Response, ctx.Request, opened.Redirect, stdhttp.StatusFound)
		return nil
	}
	scope := "private"
	if m.cfg.PublicCache {
		scope = "public"
	}
	header.Set("Cache-Control", scope+", max-age="+strconv.FormatInt(int64(m.cfg.CacheMaxAge.Seconds()), 10)+", immutable")
	header.Set("ETag", opened.ETag)
	header.Set("X-Content-Type-Options", "nosniff")
	if opened.NotModified {
		ctx.Response.WriteHeader(stdhttp.StatusNotModified)
		return nil
	}
	defer opened.Body.Body.Close()
	header.Set("Content-Type", opened.ContentType)
	header.Set("Content-Disposition", "inline")
	header.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if opened.Body.Size > 0 {
		header.Set("Content-Length", strconv.FormatInt(opened.Body.Size, 10))
	}
	ctx.Response.WriteHeader(stdhttp.StatusOK)
	_, err := io.Copy(ctx.Response, opened.Body.Body)
	return err
}

// editor answers the editor of an owner as a fragment.
func (m *Module) editor(ctx *fhttp.Context, kind Kind) error {
	who, _ := ctx.User()
	ed, err := m.avatars.Editor(ctx.Ctx(), who, Ref{Kind: kind, ID: ctx.Param("id")})
	if err != nil {
		return err
	}
	return m.writeEditor(ctx, ed)
}

// upload keeps a photo the person sent.
func (m *Module) upload(ctx *fhttp.Context, kind Kind) error {
	ref := Ref{Kind: kind, ID: ctx.Param("id")}
	who, _ := ctx.User()
	// The body is bounded before it is read, so a request cannot make the server
	// hold more than a photo is allowed to weigh.
	ctx.Request.Body = stdhttp.MaxBytesReader(ctx.Response, ctx.Request.Body, m.cfg.MaxUploadBytes+(1<<20))
	if err := ctx.Request.ParseMultipartForm(8 << 20); err != nil {
		if errors.Is(err, stdhttp.ErrNotMultipart) {
			return m.answer(ctx, who, ref, "", m.Labels().invalid("file", "file_required"))
		}
		return m.answer(ctx, who, ref, "", m.Labels().invalid("file", "multipart"))
	}
	defer func() {
		if ctx.Request.MultipartForm != nil {
			_ = ctx.Request.MultipartForm.RemoveAll()
		}
	}()
	var body io.Reader
	if headers := ctx.Request.MultipartForm.File["file"]; len(headers) > 0 {
		file, err := headers[0].Open()
		if err != nil {
			return err
		}
		defer file.Close()
		body = file
	}
	form := ctx.Request.PostForm
	_, err := m.avatars.Upload(ctx.Ctx(), who, ref, UploadRequest{Body: body, Crop: Crop{
		X: number(form.Get("crop_x")), Y: number(form.Get("crop_y")), Size: number(form.Get("crop_size")),
	}})
	return m.answer(ctx, who, ref, m.noticeFor(err, "uploaded"), err)
}

// gravatar switches the person's Gravatar on, refreshes it or switches it off.
func (m *Module) gravatar(ctx *fhttp.Context, kind Kind) error {
	ref := Ref{Kind: kind, ID: ctx.Param("id")}
	who, _ := ctx.User()
	action := GravatarAction(strings.TrimSpace(ctx.Request.PostFormValue("action")))
	_, err := m.avatars.SetGravatar(ctx.Ctx(), who, ref, action)
	notices := map[GravatarAction]string{GravatarEnable: "gravatar_enabled", GravatarRefresh: "gravatar_refreshed", GravatarDisable: "gravatar_disabled"}
	return m.answer(ctx, who, ref, m.noticeFor(err, notices[action]), err)
}

// style chooses the generated style.
func (m *Module) style(ctx *fhttp.Context, kind Kind) error {
	ref := Ref{Kind: kind, ID: ctx.Param("id")}
	who, _ := ctx.User()
	_, err := m.avatars.SetStyle(ctx.Ctx(), who, ref, Style(strings.TrimSpace(ctx.Request.PostFormValue("style"))))
	return m.answer(ctx, who, ref, m.noticeFor(err, "style_changed"), err)
}

// remove removes the photo.
func (m *Module) remove(ctx *fhttp.Context, kind Kind) error {
	ref := Ref{Kind: kind, ID: ctx.Param("id")}
	who, _ := ctx.User()
	err := m.avatars.Remove(ctx.Ctx(), who, ref)
	return m.answer(ctx, who, ref, m.noticeFor(err, "removed"), err)
}

// teamRemove removes the photo of an owner of any space, for the team.
func (m *Module) teamRemove(ctx *fhttp.Context, kind Kind) error {
	who, _ := ctx.User()
	if err := m.team.Remove(ctx.Ctx(), who, Ref{Kind: kind, ID: ctx.Param("id")}, ctx.Query("space")); err != nil {
		return err
	}
	return m.redirectBack(ctx, m.cfg.TeamPrefix)
}

// noticeFor is the sentence of a change that worked, and nothing for one that
// did not.
func (m *Module) noticeFor(err error, key string) string {
	if err != nil {
		return ""
	}
	return m.Labels().T("notice." + key)
}

// answer finishes a change. An htmx request is answered with the editor drawn
// again, carrying the sentence of what changed or the messages of what was
// refused; any other request is sent back to the page it came from, or has its
// refusal returned for the router to answer.
func (m *Module) answer(ctx *fhttp.Context, who security.Subject, ref Ref, notice string, actErr error) error {
	var refused validation.Errors
	if actErr != nil && !errors.As(actErr, &refused) {
		return actErr
	}
	if ctx.IsHTMX() && m.cfg.Render != nil {
		ed, err := m.avatars.Editor(ctx.Ctx(), who, ref)
		if err != nil {
			return err
		}
		return m.writeEditor(ctx, ed.WithNotice(notice).WithErrors(refused))
	}
	if actErr != nil {
		return actErr
	}
	return m.redirectBack(ctx, m.URLs().Editor(ref).Self)
}

// writeEditor draws the editor with Config.Render.
func (m *Module) writeEditor(ctx *fhttp.Context, ed Editor) error {
	if m.cfg.Render == nil {
		return ErrNotFound
	}
	markup, err := m.cfg.Render(ctx, ed)
	if err != nil {
		return err
	}
	header := ctx.Response.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "private, no-store")
	ctx.Response.WriteHeader(stdhttp.StatusOK)
	_, err = io.WriteString(ctx.Response, string(markup))
	return err
}

// redirectBack sends the person to the page they came from, when that page is
// on this origin, and otherwise to the fallback. The Referer is a value the
// client chose, so only its path and query are ever used.
func (m *Module) redirectBack(ctx *fhttp.Context, fallback string) error {
	if ref := ctx.Header("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Host == ctx.Request.Host && strings.HasPrefix(u.Path, "/") && !strings.HasPrefix(u.Path, "//") {
			fallback = u.RequestURI()
		}
	}
	return ctx.Redirect(fallback)
}

// number reads a fraction a form sent, and 0 for anything that is not one.
func number(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v != v {
		return 0
	}
	return v
}
