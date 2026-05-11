package parser

import "testing"

func TestGoTestCodeIsClassifiedAndEdgesReachProduction(t *testing.T) {
	src := []byte(`package service

import "testing"

func TestCreateUser(t *testing.T) {
	helper()
}

func helper() {
	CreateUser()
}
`)

	nodes := Parse(src, "service/user_test.go")
	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2", len(nodes))
	}
	nodeByName := map[string]bool{"service:service.CreateUser": true}
	for _, n := range nodes {
		if !n.IsTestCode {
			t.Fatalf("%s was not marked as test code", n.FullName)
		}
		if n.Name == "TestCreateUser" && !n.IsTestEntrypoint {
			t.Fatalf("%s was not marked as a test entrypoint", n.FullName)
		}
		nodeByName[n.FullName] = true
	}

	edges := ExtractCallEdges(src, "service/user_test.go", nodeByName)
	if !hasCallEdge(edges, "service:service.TestCreateUser", "service:service.helper") {
		t.Fatalf("missing TestCreateUser -> helper edge: %#v", edges)
	}
	if !hasCallEdge(edges, "service:service.helper", "service:service.CreateUser") {
		t.Fatalf("missing helper -> CreateUser edge: %#v", edges)
	}
}

func TestTypeScriptTestCallIsParsedAsEntrypoint(t *testing.T) {
	src := []byte(`import { saveUser } from "./users";

describe("users", () => {
  it("saves a user", () => {
    saveUser();
  });
});
`)

	nodes := Parse(src, "users.test.ts")
	nodeByName := map[string]bool{"saveUser": true}
	foundEntrypoint := false
	for _, n := range nodes {
		if !n.IsTestCode {
			t.Fatalf("%s was not marked as test code", n.FullName)
		}
		if n.Name == "saves a user" {
			foundEntrypoint = n.IsTestEntrypoint
		}
		nodeByName[n.FullName] = true
	}
	if !foundEntrypoint {
		t.Fatalf("test call was not parsed as a test entrypoint: %#v", nodes)
	}

	edges := ExtractCallEdges(src, "users.test.ts", nodeByName)
	if !hasCallEdge(edges, "users.test.saves a user", "saveUser") {
		t.Fatalf("missing test entrypoint -> saveUser edge: %#v", edges)
	}
}

func TestJavaScriptSpecFileIsTestFile(t *testing.T) {
	if !IsTestFile("__tests__/users.spec.js") {
		t.Fatal("expected __tests__/users.spec.js to be classified as a test file")
	}
	if IsTestFile("src/users.js") {
		t.Fatal("did not expect src/users.js to be classified as a test file")
	}
}

func hasCallEdge(edges []CallEdge, caller, callee string) bool {
	for _, edge := range edges {
		if edge.CallerFullName == caller && edge.CalleeFullName == callee {
			return true
		}
	}
	return false
}
