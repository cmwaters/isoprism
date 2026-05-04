package parser

import "testing"

func TestGoSelectorExternalPackageDoesNotResolveByBareSelector(t *testing.T) {
	src := []byte(`package hash

import "crypto/sha256"

func HashFromByteSlices(items [][]byte) []byte {
	return hashFromByteSlices(sha256.New(), items)
}

func hashFromByteSlices(h hash.Hash, items [][]byte) []byte {
	return nil
}
`)

	nodeByName := map[string]bool{
		"service/hash:hash.HashFromByteSlices": true,
		"service/hash:hash.hashFromByteSlices": true,
		"internal/client:client.New":           true,
	}
	edges := ExtractCallEdges(src, "service/hash/hash.go", nodeByName)

	if !hasEdge(edges, "service/hash:hash.HashFromByteSlices", "service/hash:hash.hashFromByteSlices") {
		t.Fatalf("expected local helper edge, got %#v", edges)
	}
	if hasEdge(edges, "service/hash:hash.HashFromByteSlices", "internal/client:client.New") {
		t.Fatalf("external sha256.New resolved to internal client.New: %#v", edges)
	}
}

func TestGoDuplicateNewFunctionsDoNotCollide(t *testing.T) {
	clientSrc := []byte(`package client

func New() *Client { return &Client{} }
type Client struct{}
`)
	serverSrc := []byte(`package server

func New() *Server { return &Server{} }
type Server struct{}
`)

	clientNodes := Parse(clientSrc, "internal/client/client.go")
	serverNodes := Parse(serverSrc, "internal/server/server.go")

	if !hasNode(clientNodes, "internal/client:client.New") {
		t.Fatalf("client New node missing: %#v", clientNodes)
	}
	if !hasNode(serverNodes, "internal/server:server.New") {
		t.Fatalf("server New node missing: %#v", serverNodes)
	}
	if clientNodes[0].FullName == serverNodes[0].FullName {
		t.Fatalf("duplicate New functions collided: %q", clientNodes[0].FullName)
	}
}

func TestGoFunctionAndMethodExtraction(t *testing.T) {
	src := []byte(`package service

type UserStore struct{}

func CreateUser(name string) (*User, error) { return nil, nil }

func (s *UserStore) Save(user *User) error { return nil }

type User struct{}
`)

	nodes := Parse(src, "internal/service/users.go")
	if !hasNode(nodes, "internal/service:service.CreateUser") {
		t.Fatalf("CreateUser node missing: %#v", nodes)
	}
	if !hasNode(nodes, "internal/service:service.UserStore.Save") {
		t.Fatalf("UserStore.Save node missing: %#v", nodes)
	}
	if !hasKind(nodes, "internal/service:service.UserStore", "struct") {
		t.Fatalf("UserStore struct node missing: %#v", nodes)
	}
}

func TestScriptFunctionExtractionAndCallEdges(t *testing.T) {
	src := []byte(`export function saveUser(user: User) {
  return normalizeUser(user);
}

const normalizeUser = (user: User) => user;

class UserService {
  save(user: User) {
    return saveUser(user);
  }
}
`)

	nodes := Parse(src, "web/users.ts")
	if !hasNode(nodes, "web/users.saveUser") {
		t.Fatalf("saveUser node missing: %#v", nodes)
	}
	if !hasNode(nodes, "web/users.normalizeUser") {
		t.Fatalf("normalizeUser node missing: %#v", nodes)
	}
	if !hasNode(nodes, "web/users.UserService.save") {
		t.Fatalf("class method node missing: %#v", nodes)
	}

	nodeByName := map[string]bool{
		"web/users.saveUser":      true,
		"web/users.normalizeUser": true,
	}
	edges := ExtractCallEdges(src, "web/users.ts", nodeByName)
	if !hasEdge(edges, "web/users.saveUser", "web/users.normalizeUser") {
		t.Fatalf("expected TS call edge, got %#v", edges)
	}
}

func hasNode(nodes []Node, fullName string) bool {
	for _, node := range nodes {
		if node.FullName == fullName {
			return true
		}
	}
	return false
}

func hasKind(nodes []Node, fullName, kind string) bool {
	for _, node := range nodes {
		if node.FullName == fullName && node.Kind == kind {
			return true
		}
	}
	return false
}

func hasEdge(edges []CallEdge, caller, callee string) bool {
	for _, edge := range edges {
		if edge.CallerFullName == caller && edge.CalleeFullName == callee {
			return true
		}
	}
	return false
}
