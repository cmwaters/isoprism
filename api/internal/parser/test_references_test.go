package parser

import "testing"

func TestGoTestCodeIsClassifiedAndReferencesProduction(t *testing.T) {
	src := []byte(`package service_test

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
	for _, n := range nodes {
		if !n.IsTestCode {
			t.Fatalf("%s was not marked as test code", n.FullName)
		}
	}

	refs := ExtractTestReferences(src, "service/user_test.go", map[string]bool{"CreateUser": true})
	if len(refs) != 1 {
		t.Fatalf("len(refs) = %d, want 1: %#v", len(refs), refs)
	}
	if refs[0].TestName != "TestCreateUser" || refs[0].TargetFullName != "CreateUser" {
		t.Fatalf("ref = %#v, want TestCreateUser -> CreateUser", refs[0])
	}
}

func TestTypeScriptTestReferencesProduction(t *testing.T) {
	src := []byte(`import { saveUser } from "./users";

describe("users", () => {
  it("saves a user", () => {
    saveUser();
  });
});
`)

	nodes := Parse(src, "users.test.ts")
	for _, n := range nodes {
		if !n.IsTestCode {
			t.Fatalf("%s was not marked as test code", n.FullName)
		}
	}

	refs := ExtractTestReferences(src, "users.test.ts", map[string]bool{"saveUser": true})
	if len(refs) != 1 {
		t.Fatalf("len(refs) = %d, want 1: %#v", len(refs), refs)
	}
	if refs[0].TestName != "saves a user" || refs[0].TargetFullName != "saveUser" {
		t.Fatalf("ref = %#v, want saves a user -> saveUser", refs[0])
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
