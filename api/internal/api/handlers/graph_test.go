package handlers

import "testing"

func TestBaseLookupIdentityUsesOldIdentityForRenames(t *testing.T) {
	changeType := "renamed"
	oldFullName := "BlockAPI.closeAllListeners"
	oldFilePath := "rpc/grpc/api.go"

	gotName, gotPath := baseLookupIdentity(
		"BlockAPI.closeAllListenersLocked",
		"rpc/grpc/api.go",
		&changeType,
		&oldFullName,
		&oldFilePath,
	)

	if gotName != oldFullName {
		t.Fatalf("base lookup full name = %q, want %q", gotName, oldFullName)
	}
	if gotPath != oldFilePath {
		t.Fatalf("base lookup file path = %q, want %q", gotPath, oldFilePath)
	}
}

func TestBaseLookupIdentityFallsBackWhenRenameMetadataMissing(t *testing.T) {
	changeType := "renamed"

	gotName, gotPath := baseLookupIdentity(
		"BlockAPI.closeAllListenersLocked",
		"rpc/grpc/api.go",
		&changeType,
		nil,
		nil,
	)

	if gotName != "BlockAPI.closeAllListenersLocked" {
		t.Fatalf("base lookup full name = %q", gotName)
	}
	if gotPath != "rpc/grpc/api.go" {
		t.Fatalf("base lookup file path = %q", gotPath)
	}
}

func TestBaseLookupIdentityIgnoresOldIdentityForNonRenames(t *testing.T) {
	changeType := "modified"
	oldFullName := "BlockAPI.closeAllListeners"
	oldFilePath := "rpc/grpc/old_api.go"

	gotName, gotPath := baseLookupIdentity(
		"BlockAPI.closeAllListenersLocked",
		"rpc/grpc/api.go",
		&changeType,
		&oldFullName,
		&oldFilePath,
	)

	if gotName != "BlockAPI.closeAllListenersLocked" {
		t.Fatalf("base lookup full name = %q", gotName)
	}
	if gotPath != "rpc/grpc/api.go" {
		t.Fatalf("base lookup file path = %q", gotPath)
	}
}
