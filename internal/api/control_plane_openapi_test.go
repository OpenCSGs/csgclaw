package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestControlPlaneOpenAPIMatchesRoutes(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi/control-plane.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		OpenAPI string `json:"openapi"`
		Paths   map[string]map[string]struct {
			OperationID string         `json:"operationId"`
			Responses   map[string]any `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.OpenAPI != "3.1.1" {
		t.Fatalf("OpenAPI version = %s", spec.OpenAPI)
	}
	routes := map[string]bool{}
	h := &Handler{}
	if err := chi.Walk(h.Routes(), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[method+" "+strings.TrimRight(route, "/")] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	ids := map[string]bool{}
	for path, methods := range spec.Paths {
		for method, operation := range methods {
			key := strings.ToUpper(method) + " /api/v1" + path
			if !routes[key] {
				t.Errorf("documented operation has no route: %s", key)
			}
			if operation.OperationID == "" || ids[operation.OperationID] {
				t.Errorf("missing or repeated operationId: %s", key)
			}
			if len(operation.Responses) == 0 {
				t.Errorf("missing responses: %s", key)
			}
			ids[operation.OperationID] = true
			documented[key] = true
		}
	}

	// The registration wrapper identifies the ordinary control operations in
	// this change. Check the reverse direction so adding one requires a schema.
	file, err := parser.ParseFile(token.NewFileSet(), "router.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var visit func(*ast.BlockStmt, string)
	visit = func(block *ast.BlockStmt, prefix string) {
		for _, statement := range block.List {
			expression, ok := statement.(*ast.ExprStmt)
			if !ok {
				continue
			}
			call, ok := expression.X.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				continue
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			path, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			path = strings.TrimRight(prefix+path, "/")
			if selector.Sel.Name == "Route" {
				if nested, ok := call.Args[1].(*ast.FuncLit); ok {
					visit(nested.Body, path)
				}
				continue
			}
			wrapper, ok := call.Args[1].(*ast.CallExpr)
			if !ok {
				continue
			}
			name, ok := wrapper.Fun.(*ast.Ident)
			if !ok || name.Name != "controlPlane" {
				continue
			}
			key := strings.ToUpper(selector.Sel.Name) + " " + path
			if !documented[key] {
				t.Errorf("control operation has no OpenAPI definition: %s", key)
			}
		}
	}
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "registerCoreRoutes" {
			visit(function.Body, "")
		}
	}
}
