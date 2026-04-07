package usagestats

import "testing"

func TestStoreAggregatesAndSortsRows(t *testing.T) {
	s := New()
	s.Record(Event{
		Surface:        "openai_chat",
		AccountID:      "user1@example.com",
		AccountType:    "managed",
		RequestedModel: "deepseek-chat",
		ResolvedModel:  "deepseek-chat",
		ResponseModel:  "deepseek-chat",
	})
	s.Record(Event{
		Surface:        "openai_chat",
		AccountID:      "user1@example.com",
		AccountType:    "managed",
		RequestedModel: "deepseek-chat",
		ResolvedModel:  "deepseek-chat",
		ResponseModel:  "deepseek-chat",
	})
	s.Record(Event{
		Surface:        "openai_responses",
		AccountID:      "user2@example.com",
		AccountType:    "managed",
		RequestedModel: "deepseek-reasoner",
		ResolvedModel:  "deepseek-reasoner",
		ResponseModel:  "deepseek-reasoner",
	})

	snap := s.Snapshot()
	if snap.Summary.TotalCalls != 3 {
		t.Fatalf("unexpected total calls: got %d", snap.Summary.TotalCalls)
	}
	if snap.Summary.AccountCount != 2 {
		t.Fatalf("unexpected account count: got %d", snap.Summary.AccountCount)
	}
	if snap.Summary.ModelCount != 2 {
		t.Fatalf("unexpected model count: got %d", snap.Summary.ModelCount)
	}
	if len(snap.Rows) != 2 {
		t.Fatalf("unexpected rows: got %d", len(snap.Rows))
	}
	if snap.Rows[0].Count != 2 {
		t.Fatalf("expected first row count 2, got %d", snap.Rows[0].Count)
	}
	if snap.Rows[0].AccountID != "user1@example.com" {
		t.Fatalf("unexpected first row account: %q", snap.Rows[0].AccountID)
	}
}
