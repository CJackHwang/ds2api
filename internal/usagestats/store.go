package usagestats

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Surface        string
	AccountID      string
	AccountType    string
	RequestedModel string
	ResolvedModel  string
	ResponseModel  string
}

type Row struct {
	Surface        string `json:"surface"`
	AccountID      string `json:"account_id"`
	AccountType    string `json:"account_type"`
	RequestedModel string `json:"requested_model"`
	ResolvedModel  string `json:"resolved_model"`
	ResponseModel  string `json:"response_model"`
	Count          int64  `json:"count"`
	LastCalledAt   int64  `json:"last_called_at"`
}

type Summary struct {
	TotalCalls   int64 `json:"total_calls"`
	AccountCount int   `json:"account_count"`
	ModelCount   int   `json:"model_count"`
	SurfaceCount int   `json:"surface_count"`
	RowCount     int   `json:"row_count"`
	LastCalledAt int64 `json:"last_called_at"`
}

type Snapshot struct {
	Summary Summary `json:"summary"`
	Rows    []Row   `json:"rows"`
}

type key struct {
	surface        string
	accountID      string
	accountType    string
	requestedModel string
	resolvedModel  string
	responseModel  string
}

type bucket struct {
	count        int64
	lastCalledAt int64
}

type Store struct {
	mu      sync.RWMutex
	buckets map[key]bucket
}

func New() *Store {
	return &Store{buckets: map[key]bucket{}}
}

func (s *Store) Record(evt Event) {
	if s == nil {
		return
	}
	k := key{
		surface:        strings.TrimSpace(evt.Surface),
		accountID:      strings.TrimSpace(evt.AccountID),
		accountType:    strings.TrimSpace(evt.AccountType),
		requestedModel: strings.TrimSpace(evt.RequestedModel),
		resolvedModel:  strings.TrimSpace(evt.ResolvedModel),
		responseModel:  strings.TrimSpace(evt.ResponseModel),
	}
	if k.accountID == "" {
		k.accountID = "unknown"
	}
	if k.accountType == "" {
		k.accountType = "unknown"
	}
	now := time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.buckets[k]
	cur.count++
	cur.lastCalledAt = now
	s.buckets[k] = cur
}

func (s *Store) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows := make([]Row, 0, len(s.buckets))
	accountSet := map[string]struct{}{}
	modelSet := map[string]struct{}{}
	surfaceSet := map[string]struct{}{}
	var summary Summary

	for k, v := range s.buckets {
		row := Row{
			Surface:        k.surface,
			AccountID:      k.accountID,
			AccountType:    k.accountType,
			RequestedModel: k.requestedModel,
			ResolvedModel:  k.resolvedModel,
			ResponseModel:  k.responseModel,
			Count:          v.count,
			LastCalledAt:   v.lastCalledAt,
		}
		rows = append(rows, row)
		summary.TotalCalls += v.count
		if row.AccountID != "" {
			accountSet[row.AccountID] = struct{}{}
		}
		switch {
		case row.ResolvedModel != "":
			modelSet[row.ResolvedModel] = struct{}{}
		case row.ResponseModel != "":
			modelSet[row.ResponseModel] = struct{}{}
		case row.RequestedModel != "":
			modelSet[row.RequestedModel] = struct{}{}
		}
		if row.Surface != "" {
			surfaceSet[row.Surface] = struct{}{}
		}
		if row.LastCalledAt > summary.LastCalledAt {
			summary.LastCalledAt = row.LastCalledAt
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].LastCalledAt != rows[j].LastCalledAt {
			return rows[i].LastCalledAt > rows[j].LastCalledAt
		}
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		if rows[i].AccountID != rows[j].AccountID {
			return rows[i].AccountID < rows[j].AccountID
		}
		if rows[i].ResolvedModel != rows[j].ResolvedModel {
			return rows[i].ResolvedModel < rows[j].ResolvedModel
		}
		return rows[i].Surface < rows[j].Surface
	})

	summary.AccountCount = len(accountSet)
	summary.ModelCount = len(modelSet)
	summary.SurfaceCount = len(surfaceSet)
	summary.RowCount = len(rows)
	return Snapshot{Summary: summary, Rows: rows}
}
