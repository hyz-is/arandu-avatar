package unit_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestTheApplicationIntegrationHasOneCompositionFile keeps the package's
// installation surface as explicit as its constructor. A package is composed in
// bootstrap/app.go; another bootstrap file would be another place a reader has
// to search to learn what the application contains.
func TestTheApplicationIntegrationHasOneCompositionFile(t *testing.T) {
	t.Parallel()

	root := packageRoot(t)
	guides := []string{"README.md", ".agents/skills/avatar-package/SKILL.md"}
	bootstrapFile := regexp.MustCompile(`bootstrap/[A-Za-z0-9_.-]+\.go`)

	for _, guide := range guides {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(guide)))
		if err != nil {
			t.Fatalf("reading %s: %v", guide, err)
		}
		text := string(body)
		for _, required := range []string{"bootstrap/app.go", "avatar.New(", "k.Register", "avatar.Directory", "components.Render", "aru migrate", "avatar.Assets()"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s no longer teaches the explicit composition step %q", guide, required)
			}
		}
		for _, referenced := range bootstrapFile.FindAllString(text, -1) {
			if referenced != "bootstrap/app.go" {
				t.Errorf("%s introduces a second composition file %s", guide, referenced)
			}
		}
	}
}

// TestThePackageHasOneOwnerPerResponsibility freezes the application shapes this
// package is allowed to introduce.
func TestThePackageHasOneOwnerPerResponsibility(t *testing.T) {
	t.Parallel()

	root := packageRoot(t)
	for _, path := range []string{
		"app/Actions", "app/Data", "app/Rules", "app/Support", "bootstrap/providers.go", "routes/api.go",
		"repository.go", "CLAUDE.md",
	} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path))); err == nil {
			t.Errorf("refused path %s exists in the package", path)
		} else if !os.IsNotExist(err) {
			t.Errorf("checking refused path %s: %v", path, err)
		}
	}
}

// TestNothingLinksCOrLeavesTheBinaryDynamic holds what makes the applications
// that install this build into a static, distroless image: no cgo, and none of
// the libraries that bring a C dependency for HEIC and the like.
func TestNothingLinksCOrLeavesTheBinaryDynamic(t *testing.T) {
	t.Parallel()

	forbidden := []string{"gen2brain", "libheif", "go-webp", "bimg", "govips", "imagick", "chai2010/webp"}
	for _, source := range productionGoFiles(t, packageRoot(t)) {
		for _, imported := range source.file.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			if path == "C" {
				t.Errorf("%s imports C", source.path)
			}
			for _, bad := range forbidden {
				if strings.Contains(path, bad) {
					t.Errorf("%s imports %s, which links C or makes the binary dynamic", source.path, path)
				}
			}
		}
	}
	mod, err := os.ReadFile(filepath.Join(packageRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range forbidden {
		if strings.Contains(string(mod), bad) {
			t.Errorf("go.mod requires %s", bad)
		}
	}
}

// routeVerbs are the calls that register a route on a router.
var routeVerbs = map[string]bool{
	"Action": true, "Get": true, "Post": true, "Put": true, "Patch": true, "Delete": true, "Handle": true,
}

// TestHandlersLeaveTheSubjectAndTheStatusToTheFramework holds the shape of the
// controllers. Every route is registered after RequireAuth is mounted, so no
// request reaches a handler without a session and the subject is on it; a
// handler reads it with ctx.User and nothing loads a session again. The router's
// action adapter turns an error into its status, so a handler returns the error
// and writes no refusal of its own: the one place that inspects an error is the
// answer to an htmx request, which cannot follow a redirect into a fragment, and
// the upload that turns a body it could not read into a message. And a handler
// delegates to a Service: none holds the database or constructs a query.
func TestHandlersLeaveTheSubjectAndTheStatusToTheFramework(t *testing.T) {
	t.Parallel()

	root := packageRoot(t)
	fset := token.NewFileSet()
	var register *ast.FuncDecl
	for _, name := range []string{"module.go", "handlers.go"} {
		file, err := parser.ParseFile(fset, filepath.Join(root, name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || receiverType(function) != "Module" {
				continue
			}
			if function.Name.Name == "register" {
				register = function
			}
			if name != "handlers.go" {
				continue
			}
			allowsErrors := function.Name.Name == "answer" || function.Name.Name == "upload"
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch callee := spelled(call.Fun); {
				case callee == "errors.Is" || callee == "errors.As":
					if !allowsErrors {
						t.Errorf("handlers.go: %s calls %s; the router answers an error, a handler returns it", function.Name.Name, callee)
					}
				case callee == "fhttp.Refuse" || callee == "fhttp.Reject" || callee == "stdhttp.Error" || callee == "http.Error":
					t.Errorf("handlers.go: %s calls %s, which writes a refusal the router should write", function.Name.Name, callee)
				case strings.HasSuffix(callee, ".Load") && strings.Contains(callee, "sessions"):
					t.Errorf("handlers.go: %s loads the session again; RequireAuth already did", function.Name.Name)
				case callee == "Avatars" || callee == "platform.Avatars":
					t.Errorf("handlers.go: %s builds a query; a handler asks a Service", function.Name.Name)
				}
				return true
			})
		}
	}
	if register == nil {
		t.Fatal("Module has no register method")
	}
	guarded, registered := token.NoPos, token.NoPos
	ast.Inspect(register.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch name := selectorName(call.Fun); {
		case name == "RequireAuth" && guarded == token.NoPos:
			guarded = call.Pos()
		case routeVerbs[name] && registered == token.NoPos:
			registered = call.Pos()
		}
		return true
	})
	if guarded == token.NoPos {
		t.Error("register mounts no RequireAuth, so a request with no session reaches the handlers")
	} else if registered != token.NoPos && guarded > registered {
		t.Error("register registers a route before it mounts RequireAuth")
	}
}

// spelled is a selector or an identifier as it is written: "errors.Is".
func spelled(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.SelectorExpr:
		return spelled(expression.X) + "." + expression.Sel.Name
	case *ast.Ident:
		return expression.Name
	}
	return ""
}

type parsedGoFile struct {
	path string
	file *ast.File
}

func productionGoFiles(t *testing.T, root string) []parsedGoFile {
	t.Helper()

	files := []parsedGoFile{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "testdata") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") || strings.HasSuffix(entry.Name(), ".kyse.go") {
			return nil
		}
		// The comments are kept because a build constraint is one, and a file the
		// compiler never reads is not part of what this package does. buildable,
		// in audit_test.go, is what asks.
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, parsedGoFile{path: filepath.ToSlash(relative), file: file})
		return nil
	})
	if err != nil {
		t.Fatalf("reading production Go files: %v", err)
	}
	return files
}

func receiverType(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return ""
	}
	return namedType(function.Recv.List[0].Type)
}

func namedType(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return namedType(expression.X)
	case *ast.SelectorExpr:
		return expression.Sel.Name
	case *ast.IndexExpr:
		return namedType(expression.X)
	case *ast.IndexListExpr:
		return namedType(expression.X)
	default:
		return ""
	}
}

func packageRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("finding package root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
