package manualcleanup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

const benchmarkManualActionCount = 150_000

var benchmarkSessionSink Session

func BenchmarkNewSession150K(b *testing.B) {
	plan := benchmarkManualPlan(benchmarkManualActionCount)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		session, _, err := New("manual-benchmark", plan, nil, plan.CreatedAt)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkSessionSink = session
	}
}

func BenchmarkLoadProgress150K(b *testing.B) {
	store, plan := benchmarkProgressStore(b, benchmarkManualActionCount-1)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		session, ok, err := store.Load(plan.ID)
		if err != nil || !ok {
			b.Fatalf("Load ok=%t err=%v", ok, err)
		}
		benchmarkSessionSink = session
	}
}

func BenchmarkMarkNearEnd150K(b *testing.B) {
	store, plan := benchmarkProgressStore(b, benchmarkManualActionCount-1)
	base, ok, err := store.Load(plan.ID)
	if err != nil || !ok {
		b.Fatalf("Load ok=%t err=%v", ok, err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		session := base
		session.Outcomes = append([]Outcome(nil), base.Outcomes...)
		b.StartTimer()
		if completed, err := store.Mark(&session, OutcomeDone, time.Now().UTC()); err != nil || !completed {
			b.Fatalf("Mark completed=%t err=%v", completed, err)
		}
		benchmarkSessionSink = session
	}
}

func BenchmarkLatestUnfinished150K(b *testing.B) {
	store, _ := benchmarkProgressStore(b, benchmarkManualActionCount-1)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		session, ok, err := store.LatestUnfinished()
		if err != nil || !ok {
			b.Fatalf("LatestUnfinished ok=%t err=%v", ok, err)
		}
		benchmarkSessionSink = session
	}
}

func benchmarkProgressStore(b *testing.B, progressed int) (Store, domain.CleanupPlan) {
	b.Helper()
	plan := benchmarkManualPlan(benchmarkManualActionCount)
	session, _, err := New("manual-benchmark", plan, nil, plan.CreatedAt)
	if err != nil {
		b.Fatal(err)
	}
	store := NewStore(b.TempDir())
	if err := store.Start(session); err != nil {
		b.Fatal(err)
	}
	_, progressPath := store.paths(plan.ID)
	file, err := os.Create(progressPath)
	if err != nil {
		b.Fatal(err)
	}
	writer := bufio.NewWriterSize(file, 1<<20)
	encoder := json.NewEncoder(writer)
	if err := encoder.Encode(progressEvent{ID: session.ID, At: plan.CreatedAt, Kind: "started", Position: 0, State: StateActive}); err != nil {
		b.Fatal(err)
	}
	for index := 0; index < progressed; index++ {
		event := progressEvent{
			ID:       session.ID,
			At:       plan.CreatedAt.Add(time.Duration(index+1) * time.Nanosecond),
			Kind:     string(OutcomeDone),
			ActionID: session.Actions[index].ActionID,
			Outcome:  OutcomeDone,
			Position: index + 1,
			State:    StateActive,
		}
		if err := encoder.Encode(event); err != nil {
			b.Fatal(err)
		}
	}
	if err := writer.Flush(); err != nil {
		b.Fatal(err)
	}
	if err := file.Close(); err != nil {
		b.Fatal(err)
	}
	return store, plan
}

func benchmarkManualPlan(count int) domain.CleanupPlan {
	createdAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	actions := make([]domain.CleanupAction, count)
	for index := range actions {
		username := fmt.Sprintf("demo_%06d", index)
		actions[index] = domain.CleanupAction{
			ID:                   fmt.Sprintf("action-%06d", index),
			Platform:             domain.PlatformInstagram,
			Type:                 domain.ActionUnfollow,
			TargetURL:            "https://www.instagram.com/" + username + "/",
			TargetID:             username,
			SourceActivityItemID: fmt.Sprintf("item-%06d", index),
			Status:               domain.ActionStatusPending,
			CreatedAt:            createdAt,
		}
	}
	return domain.NewCleanupPlan("plan-benchmark", domain.PlatformInstagram, "synthetic benchmark", createdAt, actions)
}
