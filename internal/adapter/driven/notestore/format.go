package notestore

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// version is the format written. The whole format stands from the start,
// sessions and batches included, so no seam drops fields it has not used
// yet.
const version = 1

type fileJSON struct {
	Version  int           `json:"version"`
	Notes    []noteJSON    `json:"notes"`
	Sessions []sessionJSON `json:"sessions"`
	Batches  []batchJSON   `json:"batches"`
}

type noteJSON struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Anchor     anchorJSON  `json:"anchor"`
	Comparison string      `json:"comparison"`
	Text       string      `json:"text"`
	State      string      `json:"state"`
	Created    time.Time   `json:"created"`
	Claim      *claimJSON  `json:"claim,omitempty"`
	Batch      string      `json:"batch,omitempty"`
	Replies    []replyJSON `json:"replies,omitempty"`
}

type anchorJSON struct {
	Path    string     `json:"path"`
	Origin  originJSON `json:"origin"`
	Blob    string     `json:"blob,omitempty"`
	Start   int        `json:"start"`
	End     int        `json:"end"`
	Lines   []string   `json:"lines"`
	Before  []string   `json:"before"`
	After   []string   `json:"after"`
	Copies  int        `json:"copies"`
	Ordinal int        `json:"ordinal"`
}

type originJSON struct {
	Kind   string `json:"kind"`
	Commit string `json:"commit,omitempty"`
}

type replyJSON struct {
	ID     string    `json:"id"`
	Author string    `json:"author"`
	Claim  string    `json:"claim,omitempty"`
	Text   string    `json:"text"`
	At     time.Time `json:"at"`
}

type claimJSON struct {
	ID      string    `json:"id"`
	Session string    `json:"session"`
	At      time.Time `json:"at"`
}

type batchJSON struct {
	ID      string    `json:"id"`
	Session string    `json:"session"`
	Notes   []string  `json:"notes"`
	At      time.Time `json:"at"`
}

type sessionJSON struct {
	ID      string    `json:"id"`
	Client  string    `json:"client"`
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
}

func encode(b note.Book) ([]byte, error) {
	f := fileJSON{Version: version, Notes: []noteJSON{}, Sessions: []sessionJSON{}, Batches: []batchJSON{}}
	for _, n := range b.Notes {
		f.Notes = append(f.Notes, encodeNote(n))
	}
	for _, s := range b.Sessions {
		f.Sessions = append(f.Sessions, sessionJSON{ID: string(s.ID), Client: s.Client, PID: s.PID, Started: s.Started})
	}
	for _, bt := range b.Batches {
		f.Batches = append(f.Batches, batchJSON{ID: string(bt.ID), Session: string(bt.Session), Notes: ids(bt.Notes), At: bt.At})
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode notes: %w", err)
	}
	return append(data, '\n'), nil
}

func encodeNote(n note.Note) noteJSON {
	a := n.Anchor
	j := noteJSON{
		ID: string(n.ID), Kind: n.Kind.String(), Comparison: n.Comparison, Text: n.Text, State: n.State.String(), Created: n.Created,
		Batch: string(n.Batch),
		Anchor: anchorJSON{
			Path: a.Path, Origin: originJSON{Kind: originName(a.Origin.Kind), Commit: a.Origin.Commit.String()}, Blob: a.Blob.String(),
			Start: a.Start, End: a.End, Lines: a.Lines, Before: a.Before, After: a.After, Copies: a.Copies, Ordinal: a.Ordinal,
		},
	}
	if c := n.Claim; c != nil {
		j.Claim = &claimJSON{ID: string(c.ID), Session: string(c.Session), At: c.At}
	}
	for _, r := range n.Replies {
		j.Replies = append(j.Replies, replyJSON{ID: string(r.ID), Author: r.Author, Claim: string(r.Claim), Text: r.Text, At: r.At})
	}
	return j
}

// decode reads a file. What it cannot read is damaged, and a version or a
// name it does not know is a newer format: either way it is no book to
// write over.
func decode(data []byte) (note.Book, error) {
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return note.Book{}, fmt.Errorf("%w: %w", note.ErrNotesDamaged, err)
	}
	if head.Version != version {
		return note.Book{}, fmt.Errorf("%w: %d", note.ErrNotesVersion, head.Version)
	}
	var f fileJSON
	if err := json.Unmarshal(data, &f); err != nil {
		return note.Book{}, fmt.Errorf("%w: %w", note.ErrNotesDamaged, err)
	}
	var b note.Book
	for _, j := range f.Notes {
		n, err := decodeNote(j)
		if err != nil {
			return note.Book{}, err
		}
		b.Notes = append(b.Notes, n)
	}
	for _, s := range f.Sessions {
		b.Sessions = append(b.Sessions, note.Session{ID: note.ID(s.ID), Client: s.Client, PID: s.PID, Started: s.Started})
	}
	for _, bt := range f.Batches {
		b.Batches = append(b.Batches, note.Batch{ID: note.ID(bt.ID), Session: note.ID(bt.Session), Notes: noteIDs(bt.Notes), At: bt.At})
	}
	return b, nil
}

func decodeNote(j noteJSON) (note.Note, error) {
	kind, okKind := parseKind(j.Kind)
	state, okState := parseState(j.State)
	origin, okOrigin := parseOrigin(j.Anchor.Origin.Kind)
	if !okKind || !okState || !okOrigin {
		return note.Note{}, fmt.Errorf("%w: note %s is %s, %s, in %s", note.ErrNotesVersion, j.ID, j.Kind, j.State, j.Anchor.Origin.Kind)
	}
	commit, err := optionalID(j.Anchor.Origin.Commit)
	if err != nil {
		return note.Note{}, err
	}
	blob, err := optionalID(j.Anchor.Blob)
	if err != nil {
		return note.Note{}, err
	}
	a := j.Anchor
	n := note.Note{
		ID: note.ID(j.ID), Kind: kind, Comparison: j.Comparison, Text: j.Text, State: state, Created: j.Created, Batch: note.ID(j.Batch),
		Anchor: note.Anchor{
			Path: a.Path, Origin: note.Origin{Kind: origin, Commit: commit}, Blob: blob,
			Start: a.Start, End: a.End, Lines: a.Lines, Before: a.Before, After: a.After, Copies: a.Copies, Ordinal: a.Ordinal,
		},
	}
	if c := j.Claim; c != nil {
		n.Claim = &note.Claim{ID: note.ID(c.ID), Session: note.ID(c.Session), At: c.At}
	}
	for _, r := range j.Replies {
		n.Replies = append(n.Replies, note.Reply{ID: note.ID(r.ID), Author: r.Author, Claim: note.ID(r.Claim), Text: r.Text, At: r.At})
	}
	return n, nil
}

func optionalID(s string) (change.ObjectID, error) {
	if s == "" {
		return change.ObjectID{}, nil
	}
	id, err := change.ParseObjectID(s)
	if err != nil {
		return change.ObjectID{}, fmt.Errorf("%w: %w", note.ErrNotesDamaged, err)
	}
	return id, nil
}

func parseKind(s string) (note.Kind, bool) {
	for _, k := range []note.Kind{note.Question, note.Finding} {
		if k.String() == s {
			return k, true
		}
	}
	return 0, false
}

func parseState(s string) (note.State, bool) {
	for _, st := range []note.State{note.Open, note.Resolved, note.Taken, note.Answered, note.Sent} {
		if st.String() == s {
			return st, true
		}
	}
	return 0, false
}

func originName(k change.SourceKind) string {
	switch k {
	case change.KindCommit:
		return "commit"
	case change.KindIndex:
		return "index"
	case change.KindWorktree:
		return "worktree"
	case change.KindPath:
	}
	return ""
}

func parseOrigin(s string) (change.SourceKind, bool) {
	for _, k := range []change.SourceKind{change.KindCommit, change.KindIndex, change.KindWorktree} {
		if originName(k) == s {
			return k, true
		}
	}
	return 0, false
}

func ids(ns []note.ID) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = string(n)
	}
	return out
}

func noteIDs(ss []string) []note.ID {
	out := make([]note.ID, len(ss))
	for i, s := range ss {
		out[i] = note.ID(s)
	}
	return out
}
