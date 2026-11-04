package presence

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeDiscordClient struct {
	mu         sync.Mutex
	activities []Activity
	logins     int
	logouts    int
}

func (f *fakeDiscordClient) Login(string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logins++
	return nil
}

func (f *fakeDiscordClient) SetActivity(a Activity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activities = append(f.activities, a)
	return nil
}

func (f *fakeDiscordClient) Logout() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logouts++
}

func (f *fakeDiscordClient) snapshot() []Activity {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Activity(nil), f.activities...)
}

func TestFormat(t *testing.T) {
	got := Format(State{Tab: "Executor", RunningBots: 7})
	want := Activity{Details: "Currently in Executor Tab", State: "Running 7 bot(s)", LargeImage: "app_logo"}
	if got != want {
		t.Fatalf("Format() = %#v, want %#v", got, want)
	}
	if got := Format(State{Tab: "Main", RunningBots: 0}).State; got != "No bots running" {
		t.Fatalf("zero-bot state = %q, want %q", got, "No bots running")
	}
}

func TestWorkerCoalescesAndThrottles(t *testing.T) {
	fake := &fakeDiscordClient{}
	s := newService(fake)
	s.sendInterval = 70 * time.Millisecond
	s.started = true
	ctx, cancel := context.WithCancel(context.Background())
	s.done = make(chan struct{})
	go s.run(ctx)
	defer func() { cancel(); <-s.done }()

	s.update(State{Tab: "Main", RunningBots: 1})
	waitFor(t, func() bool { return len(fake.snapshot()) == 1 })

	s.update(State{Tab: "List", RunningBots: 2})
	s.update(State{Tab: "Executor", RunningBots: 3})
	s.update(State{Tab: "Database", RunningBots: 4})
	time.Sleep(30 * time.Millisecond)
	if got := len(fake.snapshot()); got != 1 {
		t.Fatalf("sent %d activities before the throttle elapsed; want 1", got)
	}
	waitFor(t, func() bool { return len(fake.snapshot()) == 2 })
	activities := fake.snapshot()
	wantLatest := Activity{Details: "Currently in Database Tab", State: "Running 4 bot(s)", LargeImage: "app_logo"}
	if activities[1] != wantLatest {
		t.Fatalf("latest activity = %#v, want %#v", activities[1], wantLatest)
	}

	s.update(State{Tab: "Database", RunningBots: 4})
	time.Sleep(100 * time.Millisecond)
	if got := len(fake.snapshot()); got != 2 {
		t.Fatalf("identical state was sent again; activity count = %d", got)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for presence worker")
}
