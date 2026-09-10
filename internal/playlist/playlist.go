// Package playlist holds the queue of files to play, and the shuffle and repeat
// rules that decide what comes next.
//
// It knows nothing about mpv or about playback: it answers "what should play
// after this?" and the application acts on the answer. That keeps the ordering
// rules — which are fiddly once shuffle and repeat combine — testable without a
// media engine.
package playlist

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"playerone/internal/appdir"
)

// FileName is the playlist file's name inside the config directory.
const FileName = "playlist.json"

// Repeat describes what happens at the end of the queue, or of a track.
type Repeat string

const (
	// RepeatOff stops when the queue is exhausted.
	RepeatOff Repeat = "off"
	// RepeatAll wraps around to the start.
	RepeatAll Repeat = "all"
	// RepeatOne replays the current item forever.
	RepeatOne Repeat = "one"
)

// ParseRepeat converts a string to a Repeat, defaulting to off.
func ParseRepeat(value string) Repeat {
	switch Repeat(strings.ToLower(strings.TrimSpace(value))) {
	case RepeatAll:
		return RepeatAll
	case RepeatOne:
		return RepeatOne
	default:
		return RepeatOff
	}
}

// Item is one entry in the queue.
type Item struct {
	Path     string  `json:"path"`
	Filename string  `json:"filename"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`

	// Missing is filled in when the list is read, not stored.
	Missing bool `json:"missing"`
}

// State is the whole queue as the interface sees it.
type State struct {
	Items   []Item `json:"items"`
	Current int    `json:"current"`
	Shuffle bool   `json:"shuffle"`
	Repeat  Repeat `json:"repeat"`
}

// List is a playlist. Safe for concurrent use.
type List struct {
	path string

	mu      sync.RWMutex
	items   []Item
	current int
	shuffle bool
	repeat  Repeat

	// order is the shuffled sequence of indices. It exists so that shuffle is a
	// stable permutation rather than a random pick each time: without it,
	// "previous" could not retrace the path actually taken, and a short queue
	// would replay the same file repeatedly.
	order []int

	rng *rand.Rand
}

// New creates an empty playlist backed by dir/playlist.json.
//
// A missing or unreadable file yields an empty playlist plus an error; the
// application logs it and carries on, because a lost queue must not stop the
// player from starting.
func New(dir string) (*List, error) {
	l := &List{
		current: -1,
		repeat:  RepeatOff,
		rng:     rand.New(rand.NewSource(seed())),
	}
	if dir != "" {
		l.path = filepath.Join(dir, FileName)
	}

	if l.path == "" {
		return l, nil
	}

	raw, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return l, nil
		}
		return l, fmt.Errorf("playlist: reading %s: %w", l.path, err)
	}

	var stored State
	if err := json.Unmarshal(raw, &stored); err != nil {
		return l, fmt.Errorf("playlist: parsing %s: %w", l.path, err)
	}

	for _, item := range stored.Items {
		if item.Path == "" {
			continue
		}
		l.items = append(l.items, item)
	}
	l.shuffle = stored.Shuffle
	l.repeat = ParseRepeat(string(stored.Repeat))
	l.current = clampIndex(stored.Current, len(l.items))
	l.reshuffleLocked()

	return l, nil
}

// Default creates a playlist in the user's configuration directory.
func Default() (*List, error) {
	dir, err := appdir.Config()
	if err != nil {
		return New("")
	}
	return New(dir)
}

// Path is the playlist file's location.
func (l *List) Path() string { return l.path }

// State returns a snapshot, with Missing filled in.
func (l *List) State() State {
	l.mu.RLock()
	items := make([]Item, len(l.items))
	copy(items, l.items)
	state := State{Items: items, Current: l.current, Shuffle: l.shuffle, Repeat: l.repeat}
	l.mu.RUnlock()

	for i := range state.Items {
		_, err := os.Stat(state.Items[i].Path)
		state.Items[i].Missing = err != nil
	}
	return state
}

// Len returns the number of items.
func (l *List) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}

// Replace sets the queue to exactly these paths and selects the first.
func (l *List) Replace(paths []string) State {
	l.mu.Lock()
	l.items = itemsFor(paths)
	l.current = -1
	if len(l.items) > 0 {
		l.current = 0
	}
	l.reshuffleLocked()
	l.mu.Unlock()

	l.save()
	return l.State()
}

// Add appends paths that are not already queued, and returns the index of the
// first newly added item, or -1 when nothing was added.
//
// Duplicates are skipped because the common way to build a queue is to add a
// folder twice by accident, and a queue with the same lecture in it three times
// is never what was wanted.
func (l *List) Add(paths []string) (State, int) {
	l.mu.Lock()

	existing := make(map[string]bool, len(l.items))
	for _, item := range l.items {
		existing[key(item.Path)] = true
	}

	firstAdded := -1
	for _, item := range itemsFor(paths) {
		if existing[key(item.Path)] {
			continue
		}
		existing[key(item.Path)] = true
		if firstAdded < 0 {
			firstAdded = len(l.items)
		}
		l.items = append(l.items, item)
	}

	if l.current < 0 && len(l.items) > 0 {
		l.current = 0
	}
	l.reshuffleLocked()
	l.mu.Unlock()

	l.save()
	return l.State(), firstAdded
}

// RemoveAt drops one item.
//
// Removing the item before the current one shifts the current index down so the
// same file stays selected; removing the current one leaves the position
// pointing at whatever took its place.
func (l *List) RemoveAt(index int) State {
	l.mu.Lock()
	if index >= 0 && index < len(l.items) {
		l.items = append(l.items[:index], l.items[index+1:]...)

		switch {
		case len(l.items) == 0:
			l.current = -1
		case index < l.current:
			l.current--
		case index == l.current && l.current >= len(l.items):
			l.current = len(l.items) - 1
		}
		l.reshuffleLocked()
	}
	l.mu.Unlock()

	l.save()
	return l.State()
}

// Clear empties the queue.
func (l *List) Clear() State {
	l.mu.Lock()
	l.items = nil
	l.order = nil
	l.current = -1
	l.mu.Unlock()

	l.save()
	return l.State()
}

// Select makes an index current and returns its item.
func (l *List) Select(index int) (Item, bool) {
	l.mu.Lock()
	if index < 0 || index >= len(l.items) {
		l.mu.Unlock()
		return Item{}, false
	}
	l.current = index
	item := l.items[index]
	l.mu.Unlock()

	l.save()
	return item, true
}

// SelectPath makes the entry for a path current, adding it if it is not queued.
//
// This is how opening a file outside the playlist keeps the queue coherent: the
// file that is playing is always the current entry.
func (l *List) SelectPath(path string) State {
	l.mu.Lock()

	target := key(path)
	found := -1
	for i, item := range l.items {
		if key(item.Path) == target {
			found = i
			break
		}
	}

	if found < 0 {
		l.items = append(l.items, itemFor(path))
		found = len(l.items) - 1
		l.reshuffleLocked()
	}
	l.current = found

	l.mu.Unlock()

	l.save()
	return l.State()
}

// UpdateCurrent records what playback learned about the current item, so the
// queue can show real titles and durations rather than filenames alone.
func (l *List) UpdateCurrent(title string, duration float64) {
	l.mu.Lock()
	changed := false
	if l.current >= 0 && l.current < len(l.items) {
		item := &l.items[l.current]
		if title != "" && item.Title != title {
			item.Title = title
			changed = true
		}
		if duration > 0 && item.Duration != duration {
			item.Duration = duration
			changed = true
		}
	}
	l.mu.Unlock()

	if changed {
		l.save()
	}
}

// Restore fills in titles and durations for entries already in the queue.
//
// A saved playlist carries both, so the queue can show real names and lengths
// straight away instead of bare filenames until each file has been opened.
func (l *List) Restore(source []Item) {
	l.mu.Lock()

	byPath := make(map[string]Item, len(source))
	for _, item := range source {
		byPath[key(item.Path)] = item
	}

	changed := false
	for i := range l.items {
		src, ok := byPath[key(l.items[i].Path)]
		if !ok {
			continue
		}
		if src.Title != "" && l.items[i].Title != src.Title {
			l.items[i].Title = src.Title
			changed = true
		}
		if src.Duration > 0 && l.items[i].Duration != src.Duration {
			l.items[i].Duration = src.Duration
			changed = true
		}
	}
	l.mu.Unlock()

	if changed {
		l.save()
	}
}

// Current returns the item that should be playing.
func (l *List) Current() (Item, int, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.current < 0 || l.current >= len(l.items) {
		return Item{}, -1, false
	}
	return l.items[l.current], l.current, true
}

// Next returns the item to play after the current one.
//
// autoAdvance distinguishes reaching the end of a file from the user pressing
// the next button: repeat-one replays the file when it ends by itself, but
// pressing next must still move on, or the button would appear broken.
func (l *List) Next(autoAdvance bool) (Item, int, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.items) == 0 {
		return Item{}, -1, false
	}
	if l.current < 0 {
		l.current = 0
		return l.items[0], 0, true
	}

	if autoAdvance && l.repeat == RepeatOne {
		return l.items[l.current], l.current, true
	}

	position := l.positionOfCurrentLocked()
	next := position + 1

	if next >= len(l.items) {
		if l.repeat == RepeatOff && autoAdvance {
			return Item{}, -1, false // the queue is finished
		}
		// Wrapping reshuffles, so a repeated shuffled queue is not the same
		// order every time round.
		if l.shuffle {
			l.reshuffleLocked()
		}
		next = 0
	}

	index := l.indexAtLocked(next)
	l.current = index
	return l.items[index], index, true
}

// Previous returns the item before the current one, wrapping to the end.
func (l *List) Previous() (Item, int, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.items) == 0 {
		return Item{}, -1, false
	}
	if l.current < 0 {
		l.current = 0
		return l.items[0], 0, true
	}

	position := l.positionOfCurrentLocked() - 1
	if position < 0 {
		position = len(l.items) - 1
	}

	index := l.indexAtLocked(position)
	l.current = index
	return l.items[index], index, true
}

// HasNext reports whether the next button would do anything.
func (l *List) HasNext() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items) > 1
}

// SetShuffle turns shuffling on or off.
func (l *List) SetShuffle(on bool) State {
	l.mu.Lock()
	l.shuffle = on
	l.reshuffleLocked()
	l.mu.Unlock()

	l.save()
	return l.State()
}

// SetRepeat sets the repeat mode.
func (l *List) SetRepeat(mode Repeat) State {
	l.mu.Lock()
	l.repeat = ParseRepeat(string(mode))
	l.mu.Unlock()

	l.save()
	return l.State()
}

// Shuffle reports whether shuffling is on.
func (l *List) Shuffle() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.shuffle
}

// Repeat reports the repeat mode.
func (l *List) Repeat() Repeat {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.repeat
}

// --- ordering helpers. All require the lock. ---

// reshuffleLocked rebuilds the play order.
//
// When shuffling, the current item is moved to the front of the new order so
// that "next" continues from where the listener actually is rather than jumping
// to an unrelated point in the permutation.
func (l *List) reshuffleLocked() {
	n := len(l.items)
	l.order = make([]int, n)
	for i := range l.order {
		l.order[i] = i
	}
	if !l.shuffle || n < 2 {
		return
	}

	l.rng.Shuffle(n, func(i, j int) {
		l.order[i], l.order[j] = l.order[j], l.order[i]
	})

	if l.current < 0 {
		return
	}
	for i, index := range l.order {
		if index == l.current {
			l.order[0], l.order[i] = l.order[i], l.order[0]
			return
		}
	}
}

// positionOfCurrentLocked finds where the current item sits in the play order.
func (l *List) positionOfCurrentLocked() int {
	for position, index := range l.order {
		if index == l.current {
			return position
		}
	}
	return 0
}

// indexAtLocked maps a position in the play order to an item index.
func (l *List) indexAtLocked(position int) int {
	if position < 0 || position >= len(l.order) {
		return 0
	}
	return l.order[position]
}

// --- persistence ---

func (l *List) save() {
	if l.path == "" {
		return
	}

	l.mu.RLock()
	stored := State{Items: append([]Item(nil), l.items...), Current: l.current, Shuffle: l.shuffle, Repeat: l.repeat}
	l.mu.RUnlock()

	// Missing is derived; storing it would go stale immediately.
	for i := range stored.Items {
		stored.Items[i].Missing = false
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return
	}
	_ = appdir.WriteFileAtomic(l.path, append(data, '\n'), 0o644)
}

// --- construction helpers ---

func itemsFor(paths []string) []Item {
	items := make([]Item, 0, len(paths))
	seen := make(map[string]bool, len(paths))

	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		item := itemFor(p)
		if seen[key(item.Path)] {
			continue
		}
		seen[key(item.Path)] = true
		items = append(items, item)
	}
	return items
}

func itemFor(path string) Item {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return Item{Path: abs, Filename: filepath.Base(abs)}
}

// key normalises a path for comparison. Windows paths are case-insensitive, so
// the same file added twice with different casing must count as one entry.
func key(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return strings.ToLower(filepath.Clean(path))
}

func clampIndex(index, length int) int {
	if length == 0 {
		return -1
	}
	if index < 0 || index >= length {
		return 0
	}
	return index
}

// SortPaths orders paths the way a file manager would, so a folder of numbered
// lessons queues in lesson order rather than in whatever order the filesystem
// returned them.
func SortPaths(paths []string) []string {
	out := append([]string(nil), paths...)
	sort.Slice(out, func(i, j int) bool {
		return naturalLess(filepath.Base(out[i]), filepath.Base(out[j]))
	})
	return out
}

// naturalLess compares strings so that "lesson2" sorts before "lesson10".
func naturalLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)

	i, j := 0, 0
	for i < len(la) && j < len(lb) {
		if isDigit(la[i]) && isDigit(lb[j]) {
			// Compare whole runs of digits numerically.
			si, sj := i, j
			for i < len(la) && isDigit(la[i]) {
				i++
			}
			for j < len(lb) && isDigit(lb[j]) {
				j++
			}

			na := strings.TrimLeft(la[si:i], "0")
			nb := strings.TrimLeft(lb[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}

		if la[i] != lb[j] {
			return la[i] < lb[j]
		}
		i++
		j++
	}
	return len(la)-i < len(lb)-j
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// seed produces a starting point for the shuffle.
//
// Shuffling only needs to look unpredictable to a listener, so the clock is a
// perfectly good source here and avoids pulling in crypto/rand for something
// that is not a security decision.
func seed() int64 {
	return time.Now().UnixNano()
}
