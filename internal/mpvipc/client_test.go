package mpvipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMPV plays the server half of the IPC protocol over an in-memory pipe.
// Every request is answered by respond, which the individual test supplies.
type fakeMPV struct {
	conn    net.Conn
	scanner *bufio.Scanner

	mu       sync.Mutex
	received [][]any
	// raw keeps the exact bytes each request arrived as. The decoded form
	// cannot answer questions about number formatting, because decoding to
	// float64 and re-encoding erases the difference between 2 and 2.0.
	raw [][]byte
}

// startFakeMPV wires a Client to a scripted server. respond is called for each
// decoded request and returns the raw JSON line to reply with; returning an
// empty string sends nothing, which models an unanswered command.
func startFakeMPV(t *testing.T, onEvent EventHandler, respond func(f *fakeMPV, req request) string) (*Client, *fakeMPV) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	f := &fakeMPV{conn: serverConn, scanner: bufio.NewScanner(serverConn)}

	go func() {
		for f.scanner.Scan() {
			var req request
			if err := json.Unmarshal(f.scanner.Bytes(), &req); err != nil {
				continue
			}

			line := make([]byte, len(f.scanner.Bytes()))
			copy(line, f.scanner.Bytes())

			f.mu.Lock()
			f.received = append(f.received, req.Command)
			f.raw = append(f.raw, line)
			f.mu.Unlock()

			if reply := respond(f, req); reply != "" {
				_, _ = f.conn.Write([]byte(reply + "\n"))
			}
		}
	}()

	c := newClient(clientConn, onEvent)
	t.Cleanup(func() { _ = c.Close() })
	return c, f
}

// push sends an unsolicited line, modelling an mpv event.
func (f *fakeMPV) push(line string) {
	_, _ = f.conn.Write([]byte(line + "\n"))
}

// lastRaw returns the bytes of the most recent request, or "" if none.
func (f *fakeMPV) lastRaw() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.raw) == 0 {
		return ""
	}
	return string(f.raw[len(f.raw)-1])
}

func (f *fakeMPV) commands() [][]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]any, len(f.received))
	copy(out, f.received)
	return out
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestCommandReturnsData(t *testing.T) {
	c, _ := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"success","data":8309.237,"request_id":` + itoa(req.RequestID) + `}`
	})

	data, err := c.Command(testCtx(t), "get_property", "duration")
	if err != nil {
		t.Fatalf("Command returned error: %v", err)
	}
	if string(data) != "8309.237" {
		t.Errorf("data = %s, want 8309.237", data)
	}
}

func TestCommandSendsExpectedPayload(t *testing.T) {
	c, f := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"success","request_id":` + itoa(req.RequestID) + `}`
	})

	if _, err := c.Command(testCtx(t), "set_property", "pause", true); err != nil {
		t.Fatalf("Command returned error: %v", err)
	}

	cmds := f.commands()
	if len(cmds) != 1 {
		t.Fatalf("server saw %d commands, want 1", len(cmds))
	}
	if cmds[0][0] != "set_property" || cmds[0][1] != "pause" || cmds[0][2] != true {
		t.Errorf("server saw %v, want [set_property pause true]", cmds[0])
	}
}

func TestCommandSurfacesMPVError(t *testing.T) {
	c, _ := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"property not found","request_id":` + itoa(req.RequestID) + `}`
	})

	_, err := c.Command(testCtx(t), "get_property", "nonsense")
	if err == nil {
		t.Fatal("expected an error when mpv rejects the command")
	}

	var ce *CommandError
	if !errors.As(err, &ce) {
		t.Fatalf("error is %T, want *CommandError", err)
	}
	if ce.Reason != "property not found" {
		t.Errorf("Reason = %q, want %q", ce.Reason, "property not found")
	}
}

// Replies must be matched by request_id, not by arrival order. This is the bug
// that concurrent property reads would otherwise hit.
func TestConcurrentCommandsMatchByRequestID(t *testing.T) {
	var held []request
	var mu sync.Mutex
	release := make(chan struct{})

	c, _ := startFakeMPV(t, nil, func(f *fakeMPV, req request) string {
		mu.Lock()
		held = append(held, req)
		n := len(held)
		mu.Unlock()

		if n < 2 {
			return "" // hold the first request back
		}

		// Answer out of order: the second request first, then the first.
		go func() {
			mu.Lock()
			reqs := append([]request(nil), held...)
			mu.Unlock()

			f.push(`{"error":"success","data":"second","request_id":` + itoa(reqs[1].RequestID) + `}`)
			f.push(`{"error":"success","data":"first","request_id":` + itoa(reqs[0].RequestID) + `}`)
			close(release)
		}()
		return ""
	})

	type result struct {
		data string
		err  error
	}
	first := make(chan result, 1)

	go func() {
		d, err := c.Command(testCtx(t), "first")
		first <- result{string(d), err}
	}()

	// Ensure the first command is in flight before issuing the second.
	time.Sleep(50 * time.Millisecond)

	second, err := c.Command(testCtx(t), "second")
	if err != nil {
		t.Fatalf("second Command: %v", err)
	}
	if string(second) != `"second"` {
		t.Errorf("second command got %s, want \"second\"", second)
	}

	<-release
	got := <-first
	if got.err != nil {
		t.Fatalf("first Command: %v", got.err)
	}
	if got.data != `"first"` {
		t.Errorf("first command got %s, want \"first\"", got.data)
	}
}

func TestCommandRespectsContextCancellation(t *testing.T) {
	c, _ := startFakeMPV(t, nil, func(*fakeMPV, request) string {
		return "" // never answer
	})

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	_, err := c.Command(ctx, "get_property", "duration")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

// A caller that gave up must not leave a pending slot behind, or the reader
// goroutine will block forever delivering to nobody.
func TestAbandonedRequestDoesNotWedgeReader(t *testing.T) {
	var seen []request
	var mu sync.Mutex

	c, f := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		mu.Lock()
		seen = append(seen, req)
		n := len(seen)
		mu.Unlock()
		if n == 1 {
			return "" // let the first request time out
		}
		return `{"error":"success","data":"ok","request_id":` + itoa(req.RequestID) + `}`
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if _, err := c.Command(ctx, "ignored"); err == nil {
		t.Fatal("expected the first command to time out")
	}

	// Now deliver the abandoned reply. The reader must discard it.
	mu.Lock()
	abandoned := seen[0].RequestID
	mu.Unlock()
	f.push(`{"error":"success","data":"late","request_id":` + itoa(abandoned) + `}`)

	// A subsequent command must still work.
	data, err := c.Command(testCtx(t), "still-alive")
	if err != nil {
		t.Fatalf("command after abandoned reply: %v", err)
	}
	if string(data) != `"ok"` {
		t.Errorf("data = %s, want \"ok\"", data)
	}
}

func TestEventsAreDispatched(t *testing.T) {
	events := make(chan Event, 8)
	_, f := startFakeMPV(t, func(e Event) { events <- e }, func(*fakeMPV, request) string {
		return ""
	})

	f.push(`{"event":"property-change","id":3,"name":"time-pos","data":42.5}`)

	select {
	case e := <-events:
		if e.Name != "property-change" {
			t.Errorf("Name = %q, want property-change", e.Name)
		}
		if e.Property != "time-pos" {
			t.Errorf("Property = %q, want time-pos", e.Property)
		}
		if string(e.Data) != "42.5" {
			t.Errorf("Data = %s, want 42.5", e.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the event")
	}
}

// A property that mpv reports as null must arrive as nil Data, not as a zero
// value, so callers can tell "unset" from "zero".
func TestNullPropertyDataIsNil(t *testing.T) {
	events := make(chan Event, 4)
	_, f := startFakeMPV(t, func(e Event) { events <- e }, func(*fakeMPV, request) string {
		return ""
	})

	f.push(`{"event":"property-change","id":1,"name":"time-pos","data":null}`)

	select {
	case e := <-events:
		if len(e.Data) != 0 && string(e.Data) != "null" {
			t.Errorf("Data = %s, want nil or null", e.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the event")
	}
}

func TestMalformedLineDoesNotKillSession(t *testing.T) {
	c, f := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"success","data":"fine","request_id":` + itoa(req.RequestID) + `}`
	})

	f.push(`{not json at all`)
	f.push(`}}}`)

	data, err := c.Command(testCtx(t), "get_property", "pause")
	if err != nil {
		t.Fatalf("command after malformed lines: %v", err)
	}
	if string(data) != `"fine"` {
		t.Errorf("data = %s, want \"fine\"", data)
	}
}

// mpv's chapter-list on a long tutorial easily exceeds bufio's 64 KiB default
// line limit; the scanner must be sized for it.
func TestVeryLongLineIsRead(t *testing.T) {
	big := strings.Repeat("chapter title padding ", 20000) // ~440 KB

	c, _ := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		payload, _ := json.Marshal(big)
		return `{"error":"success","data":` + string(payload) + `,"request_id":` + itoa(req.RequestID) + `}`
	})

	data, err := c.Command(testCtx(t), "get_property", "chapter-list")
	if err != nil {
		t.Fatalf("Command on a long reply: %v", err)
	}

	var got string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decoding long reply: %v", err)
	}
	if got != big {
		t.Errorf("long reply was truncated: got %d bytes, want %d", len(got), len(big))
	}
}

func TestConnectionLossReleasesWaitersAndClosesDone(t *testing.T) {
	c, f := startFakeMPV(t, nil, func(*fakeMPV, request) string { return "" })

	errCh := make(chan error, 1)
	go func() {
		_, err := c.Command(context.Background(), "get_property", "duration")
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond)
	_ = f.conn.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected an error once the connection dropped")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a pending command was not released when the connection dropped")
	}

	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done was not closed when the connection dropped")
	}
}

func TestCommandAfterCloseReturnsErrClosed(t *testing.T) {
	c, _ := startFakeMPV(t, nil, func(*fakeMPV, request) string { return "" })

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := c.Command(context.Background(), "get_property", "pause")
	if err == nil {
		t.Fatal("expected an error after Close")
	}
}

func TestDoubleCloseIsSafe(t *testing.T) {
	c, _ := startFakeMPV(t, nil, func(*fakeMPV, request) string { return "" })

	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestGetPropertyDecodesIntoTarget(t *testing.T) {
	c, _ := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"success","data":{"title":"Intro","time":180},"request_id":` + itoa(req.RequestID) + `}`
	})

	var got struct {
		Title string  `json:"title"`
		Time  float64 `json:"time"`
	}
	if err := c.GetProperty(testCtx(t), "chapter-metadata", &got); err != nil {
		t.Fatalf("GetProperty: %v", err)
	}
	if got.Title != "Intro" || got.Time != 180 {
		t.Errorf("got %+v, want {Intro 180}", got)
	}
}

func TestGetPropertyNullIsUnavailable(t *testing.T) {
	c, _ := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"success","data":null,"request_id":` + itoa(req.RequestID) + `}`
	})

	var out float64
	err := c.GetProperty(testCtx(t), "duration", &out)
	if err == nil {
		t.Fatal("expected an error for a null property")
	}
	if !IsUnavailable(err) {
		t.Errorf("IsUnavailable(%v) = false, want true", err)
	}
}

func TestIsUnavailable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"null property", &PropertyUnavailableError{Property: "duration"}, true},
		{"mpv says unavailable", &CommandError{Reason: "property unavailable"}, true},
		{"mpv says not found", &CommandError{Reason: "property not found"}, true},
		{"a real failure", &CommandError{Reason: "invalid parameter"}, false},
		{"unrelated error", errors.New("boom"), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUnavailable(tc.err); got != tc.want {
				t.Errorf("IsUnavailable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
