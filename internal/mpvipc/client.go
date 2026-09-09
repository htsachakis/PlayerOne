// Package mpvipc speaks mpv's JSON IPC protocol over a Windows named pipe.
//
// It is a transport and nothing more: it knows about requests, replies and
// events, but nothing about players, chapters or subtitles. That separation is
// what keeps mpv's wire format from leaking through the rest of the codebase.
//
// Protocol reference: mpv's JSON IPC is newline-delimited JSON in both
// directions. A request carries a request_id which the matching reply echoes;
// anything with an "event" key is an unsolicited event.
package mpvipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned once the client has been shut down.
var ErrClosed = errors.New("mpvipc: client is closed")

// request is one outgoing command.
type request struct {
	Command   []any `json:"command"`
	RequestID int64 `json:"request_id"`
	Async     bool  `json:"async,omitempty"`
}

// response is one incoming line. mpv multiplexes replies and events on the same
// stream, so both shapes are decoded into this single struct and told apart by
// which fields are populated.
type response struct {
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	RequestID int64           `json:"request_id"`

	Event string          `json:"event"`
	ID    int64           `json:"id"`
	Name  string          `json:"name"`
	}

// Event is a decoded mpv event handed to the client's event handler.
type Event struct {
	// Name is mpv's event name, for example "property-change" or "end-file".
	Name string
	// Property is set for property-change events.
	Property string
	// Data is the raw payload; nil when mpv sent JSON null (an unavailable
	// property), which callers must distinguish from a zero value.
	Data json.RawMessage
	// Raw is the entire event object, for events with fields beyond the above.
	Raw json.RawMessage
}

// EventHandler receives events. It is called from the reader goroutine, so it
// must not block and must not call back into the client synchronously.
type EventHandler func(Event)

// Client is a connected mpv IPC session.
type Client struct {
	conn    net.Conn
	writer  *bufio.Writer
	writeMu sync.Mutex

	nextID atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]chan response
	closed    bool

	onEvent EventHandler

	// closeOnce guards Close so a reader-detected failure and an explicit Close
	// cannot both tear the client down.
	closeOnce sync.Once
	done      chan struct{}
	readErr   atomic.Pointer[error]

	wg sync.WaitGroup
}

// Dial connects to an mpv IPC pipe, retrying until ctx expires.
//
// Retrying is essential rather than defensive: mpv creates the pipe some
// milliseconds after the process starts, so the first several attempts by a
// freshly spawned player legitimately fail.
func Dial(ctx context.Context, pipe string, onEvent EventHandler) (*Client, error) {
	var lastErr error

	for {
		conn, err := dialPipe(ctx, pipe)
		if err == nil {
			return newClient(conn, onEvent), nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("mpvipc: connecting to %s: %w (last attempt: %v)", pipe, ctx.Err(), lastErr)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func newClient(conn net.Conn, onEvent EventHandler) *Client {
	c := &Client{
		conn:    conn,
		writer:  bufio.NewWriter(conn),
		pending: make(map[int64]chan response),
		onEvent: onEvent,
		done:    make(chan struct{}),
	}

	c.wg.Add(1)
	go c.readLoop()

	return c
}

// Done is closed when the connection drops or the client is closed. The player
// watches it to notice that mpv has exited.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err reports why the connection ended, or nil if it was closed deliberately.
func (c *Client) Err() error {
	if p := c.readErr.Load(); p != nil {
		return *p
	}
	return nil
}

// readLoop demultiplexes replies and events until the connection ends.
func (c *Client) readLoop() {
	defer c.wg.Done()

	scanner := bufio.NewScanner(c.conn)
	// mpv can emit long lines: track-list on a file with many tracks, and
	// chapter-list on the 59-chapter fixture, both exceed the 64 KiB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp response
		if err := json.Unmarshal(line, &resp); err != nil {
			// A single malformed line must not kill the session.
			continue
		}

		if resp.Event != "" {
			c.dispatchEvent(resp, line)
			continue
		}

		c.deliverReply(resp)
	}

	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	c.failAll(err)
}

func (c *Client) dispatchEvent(resp response, line []byte) {
	if c.onEvent == nil {
		return
	}

	// The scanner reuses its buffer, so the payload must be copied before it
	// escapes this goroutine.
	raw := make([]byte, len(line))
	copy(raw, line)

	var data json.RawMessage
	if len(resp.Data) > 0 {
		data = append(json.RawMessage(nil), resp.Data...)
	}

	c.onEvent(Event{
		Name:     resp.Event,
		Property: resp.Name,
		Data:     data,
		Raw:      raw,
	})
}

func (c *Client) deliverReply(resp response) {
	c.pendingMu.Lock()
	ch, ok := c.pending[resp.RequestID]
	if ok {
		delete(c.pending, resp.RequestID)
	}
	c.pendingMu.Unlock()

	if !ok {
		return // a reply to a request that was already abandoned
	}

	// Copy before sending: the scanner's buffer is reused on the next line.
	if len(resp.Data) > 0 {
		resp.Data = append(json.RawMessage(nil), resp.Data...)
	}
	ch <- resp
}

// failAll releases every waiting caller when the connection ends.
func (c *Client) failAll(err error) {
	c.pendingMu.Lock()
	pending := c.pending
	c.pending = make(map[int64]chan response)
	c.closed = true
	c.pendingMu.Unlock()

	for id, ch := range pending {
		ch <- response{Error: fmt.Sprintf("connection lost: %v", err), RequestID: id}
	}

	c.readErr.CompareAndSwap(nil, &err)
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

// Command sends an mpv command and waits for its reply.
//
// The returned bytes are the reply's "data" field, which is nil for commands
// that return nothing.
func (c *Client) Command(ctx context.Context, args ...any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan response, 1)

	c.pendingMu.Lock()
	if c.closed {
		c.pendingMu.Unlock()
		return nil, ErrClosed
	}
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.write(request{Command: args, RequestID: id}); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != "" && resp.Error != "success" {
			return nil, &CommandError{Command: args, Reason: resp.Error}
		}
		return resp.Data, nil

	case <-ctx.Done():
		// Abandon the slot so a late reply is discarded rather than delivered
		// to a caller that has moved on.
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("mpvipc: command %v: %w", args, ctx.Err())
	}
}

func (c *Client) write(req request) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mpvipc: encoding command: %w", err)
	}
	payload = append(payload, '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := c.writer.Write(payload); err != nil {
		return fmt.Errorf("mpvipc: writing command: %w", err)
	}
	if err := c.writer.Flush(); err != nil {
		return fmt.Errorf("mpvipc: flushing command: %w", err)
	}
	return nil
}

// GetProperty reads an mpv property into out.
func (c *Client) GetProperty(ctx context.Context, name string, out any) error {
	data, err := c.Command(ctx, "get_property", name)
	if err != nil {
		return err
	}
	if len(data) == 0 || string(data) == "null" {
		return &PropertyUnavailableError{Property: name}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("mpvipc: decoding property %q: %w", name, err)
	}
	return nil
}

// SetProperty writes an mpv property.
func (c *Client) SetProperty(ctx context.Context, name string, value any) error {
	_, err := c.Command(ctx, "set_property", name, value)
	return err
}

// ObserveProperty asks mpv to push property-change events for name. The id is
// echoed on each event so a caller can correlate without string comparison.
func (c *Client) ObserveProperty(ctx context.Context, id int64, name string) error {
	_, err := c.Command(ctx, "observe_property", id, name)
	return err
}

// Close shuts the client down and waits for the reader goroutine to finish.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
	c.wg.Wait()

	c.pendingMu.Lock()
	c.closed = true
	c.pendingMu.Unlock()

	return nil
}

// CommandError is an error reported by mpv itself, as opposed to a transport
// failure. Property lookups on an idle player produce these routinely, so
// callers are expected to check for them rather than treat them as fatal.
type CommandError struct {
	Command []any
	Reason  string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("mpv rejected command %v: %s", e.Command, e.Reason)
}

// PropertyUnavailableError means mpv returned null: the property exists but has
// no value right now, for instance duration before a file is loaded.
type PropertyUnavailableError struct {
	Property string
}

func (e *PropertyUnavailableError) Error() string {
	return fmt.Sprintf("mpv property %q is not available", e.Property)
}

// IsUnavailable reports whether err means "no value right now" rather than a
// real failure. Property probes use it to stay quiet during startup.
func IsUnavailable(err error) bool {
	var pu *PropertyUnavailableError
	if errors.As(err, &pu) {
		return true
	}
	var ce *CommandError
	if errors.As(err, &ce) {
		return ce.Reason == "property unavailable" || ce.Reason == "property not found"
	}
	return false
}
