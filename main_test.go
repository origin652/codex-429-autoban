package main

import (
	"encoding/json"
	"testing"
	"time"

	"codex-429-autoban/cpasdk/pluginapi"
)

func schedulerResponseForTest(t *testing.T, candidates []pluginapi.SchedulerAuthCandidate) pluginapi.SchedulerPickResponse {
	t.Helper()
	rawRequest, err := json.Marshal(pluginapi.SchedulerPickRequest{Candidates: candidates})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rawResponse, err := handleSchedulerPick(rawRequest)
	if err != nil {
		t.Fatalf("handleSchedulerPick: %v", err)
	}
	var wrapped envelope
	if err := json.Unmarshal(rawResponse, &wrapped); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !wrapped.OK {
		t.Fatalf("plugin returned error: %s", wrapped.Error.Message)
	}
	var response pluginapi.SchedulerPickResponse
	if err := json.Unmarshal(wrapped.Result, &response); err != nil {
		t.Fatalf("unmarshal scheduler response: %v", err)
	}
	return response
}

func TestSchedulerDoesNotOverrideHostWhenNothingIsBanned(t *testing.T) {
	banStore = banState{}
	response := schedulerResponseForTest(t, []pluginapi.SchedulerAuthCandidate{
		{ID: "codex-1", Provider: providerCodex, Priority: 10},
		{ID: "codex-2", Provider: providerCodex, Priority: 10},
	})
	if response.Handled || response.DelegateBuiltin != "" || response.AuthID != "" {
		t.Fatalf("unexpected scheduling override: %+v", response)
	}
}

func TestSchedulerApproximatesFillFirstAfterBan(t *testing.T) {
	banStore = banState{}
	banStore.set("codex-banned", banEntry{ResetAt: time.Now().Add(time.Hour), Window: "5h"})
	response := schedulerResponseForTest(t, []pluginapi.SchedulerAuthCandidate{
		{ID: "codex-banned", Provider: providerCodex, Priority: 100},
		{ID: "codex-z", Provider: providerCodex, Priority: 10},
		{ID: "codex-a", Provider: providerCodex, Priority: 10},
		{ID: "codex-low", Provider: providerCodex, Priority: 1},
	})
	if !response.Handled || response.DelegateBuiltin != "" {
		t.Fatalf("response must make a direct fallback selection: %+v", response)
	}
	if response.AuthID != "codex-a" {
		t.Fatalf("AuthID = %q, want deterministic highest-priority first-fill approximation codex-a", response.AuthID)
	}
}

func TestSchedulerAllBannedDeclinesSelection(t *testing.T) {
	banStore = banState{}
	banStore.set("codex-banned", banEntry{ResetAt: time.Now().Add(time.Hour), Window: "5h"})
	response := schedulerResponseForTest(t, []pluginapi.SchedulerAuthCandidate{{ID: "codex-banned", Provider: providerCodex}})
	if response.Handled || response.AuthID != "" {
		t.Fatalf("all-banned pool must be returned to stock CPA: %+v", response)
	}
}

func TestSchedulerExpiresBanAndReturnsControlToHost(t *testing.T) {
	banStore = banState{}
	banStore.set("codex-expired", banEntry{ResetAt: time.Now().Add(-time.Second), Window: "5h"})
	response := schedulerResponseForTest(t, []pluginapi.SchedulerAuthCandidate{{ID: "codex-expired", Provider: providerCodex}})
	if response.Handled || response.AuthID != "" {
		t.Fatalf("expired ban must not alter host scheduling: %+v", response)
	}
	if _, banned := banStore.lookup("codex-expired"); banned {
		t.Fatal("expired ban was not cleared")
	}
}
