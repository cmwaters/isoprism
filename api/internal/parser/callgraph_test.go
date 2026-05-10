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

func TestGoFieldChainResolvesThroughImportedStructFields(t *testing.T) {
	files := map[string][]byte{
		"rpc/grpc/api.go": []byte(`package coregrpc

import (
	"context"
	core "github.com/cometbft/cometbft/rpc/core"
)

type BlockAPI struct {
	env *core.Environment
}

func (blockAPI *BlockAPI) Stop(ctx context.Context) error {
	return blockAPI.env.EventBus.Unsubscribe(ctx, "sub", "query")
}
`),
		"rpc/core/env.go": []byte(`package core

import eventstypes "github.com/cometbft/cometbft/types"

type Environment struct {
	EventBus *eventstypes.EventBus
}
`),
		"types/event_bus.go": []byte(`package types

import "context"

type EventBus struct{}

func (b *EventBus) Unsubscribe(ctx context.Context, subscriber string, query string) error {
	return nil
}
`),
	}
	nodeByName := nodesByName(files)
	index := BuildResolverIndex(files, nodeByName)
	edges := ExtractCallEdgesWithResolver(files["rpc/grpc/api.go"], "rpc/grpc/api.go", index)

	if !hasEdge(edges, "rpc/grpc:coregrpc.BlockAPI.Stop", "types:types.EventBus.Unsubscribe") {
		t.Fatalf("expected field-chain edge, got %#v", edges)
	}
}

func TestGoReceiverCallResolvesFromParameterType(t *testing.T) {
	files := map[string][]byte{
		"service/service.go": []byte(`package service

type Client struct{}

func (c *Client) Save() {}

func Run(client *Client) {
	client.Save()
}
`),
	}
	nodeByName := nodesByName(files)
	index := BuildResolverIndex(files, nodeByName)
	edges := ExtractCallEdgesWithResolver(files["service/service.go"], "service/service.go", index)

	if !hasEdge(edges, "service:service.Run", "service:service.Client.Save") {
		t.Fatalf("expected parameter receiver edge, got %#v", edges)
	}
}

func TestGoFieldChainDoesNotResolveAmbiguousImportedType(t *testing.T) {
	files := map[string][]byte{
		"app/app.go": []byte(`package app

import "example.com/project/shared"

type Service struct {
	store *shared.Store
}

func (s *Service) Run() {
	s.store.Save()
}
`),
		"shared/store.go": []byte(`package shared

type Store struct{}
`),
		"other/shared/store.go": []byte(`package shared

type Store struct{}

func (s *Store) Save() {}
`),
	}
	nodeByName := nodesByName(files)
	index := BuildResolverIndex(files, nodeByName)
	edges := ExtractCallEdgesWithResolver(files["app/app.go"], "app/app.go", index)

	if hasEdge(edges, "app:app.Service.Run", "other/shared:shared.Store.Save") {
		t.Fatalf("ambiguous imported type resolved to unrelated Store.Save: %#v", edges)
	}
}

func TestGoFieldChainRequiresKnownFieldType(t *testing.T) {
	files := map[string][]byte{
		"service/service.go": []byte(`package service

type Service struct {
	client any
}

func (s *Service) Run() {
	s.client.Save()
}
`),
		"service/client.go": []byte(`package service

func Save() {}
`),
	}
	nodeByName := nodesByName(files)
	index := BuildResolverIndex(files, nodeByName)
	edges := ExtractCallEdgesWithResolver(files["service/service.go"], "service/service.go", index)

	if hasEdge(edges, "service:service.Service.Run", "service:service.Save") {
		t.Fatalf("unknown field type resolved by method name only: %#v", edges)
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

func TestScriptMemberCallsDoNotResolveByPropertyName(t *testing.T) {
	src := []byte(`export function run(client: Client) {
  return client.save();
}
`)

	nodeByName := map[string]bool{
		"web/users.run":  true,
		"web/users.save": true,
	}
	edges := ExtractCallEdges(src, "web/users.ts", nodeByName)
	if hasEdge(edges, "web/users.run", "web/users.save") {
		t.Fatalf("member call resolved by property name: %#v", edges)
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

func nodesByName(files map[string][]byte) map[string]bool {
	out := map[string]bool{}
	for filePath, src := range files {
		for _, node := range Parse(src, filePath) {
			out[node.FullName] = true
		}
	}
	return out
}
