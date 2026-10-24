package hover

import (
	"seam/internal/domain/change"
	"seam/internal/domain/lang"
)

// DocVersion counts the texts a document has had in a session. It rises
// exactly when the text changes, so an answer to an older version can be
// told apart from one to the text on screen.
type DocVersion uint64

// DocKey names a document: the file at Path, relative to the repository
// root in slash form, in the worktree if Commit is zero, else as Commit has
// it, served from a checkout of that commit.
type DocKey struct {
	Commit change.ObjectID
	Path   string
}

// Document is the text a language server is to see for a file: a side of
// the diff on screen.
type Document struct {
	Commit  change.ObjectID
	Path    string
	Lang    lang.Lang
	Version DocVersion
	Text    string
}

// Key names the document.
func (d Document) Key() DocKey { return DocKey{Commit: d.Commit, Path: d.Path} }

// MaxLine is the longest line, in bytes, that hovers ask about. Past it
// the text is likely generated or minified, where an answer helps little
// and finding the grapheme and the identifier costs on every move.
const MaxLine = 64 << 10

// ReqID names a hover request. The viewer hands out strictly increasing
// ones before it asks, so an answer finds its request whenever it comes.
type ReqID uint64

// Position is where a hover asks: in the document of Commit and Path as of
// Version, at a byte offset into a line, both counted from zero.
type Position struct {
	Req     ReqID
	Commit  change.ObjectID
	Path    string
	Version DocVersion
	Line    int
	Byte    int
}

// Key names the document asked about.
func (p Position) Key() DocKey { return DocKey{Commit: p.Commit, Path: p.Path} }

// Submit is how a hover request was taken.
type Submit int

const (
	// Sent means the answer will come as an update.
	Sent Submit = iota + 1
	// NotReady means the server is starting; ask again once it is Ready.
	NotReady
	// Unsupported means no server hovers over the document.
	Unsupported
	// Stale means Version is not the one last synced for the document.
	Stale
	// Busy means the server has too much waiting; ask again later.
	Busy
	// Failed means the server has failed and takes no more requests.
	Failed
)

// ServerStatus is the state of a server instance.
type ServerStatus int

// The states of a server instance; see the plan's state machine.
const (
	Missing ServerStatus = iota + 1
	Starting
	Ready
	ServerFailed
	Stopped
)

// Outcome is how a hover request ended.
type Outcome int

const (
	// Text is an answer with content.
	Text Outcome = iota + 1
	// Empty is an answer without content.
	Empty
	// Error is a request that failed, by the server's answer or its end.
	Error
	// TimedOut is a request the server did not answer in time.
	TimedOut
)

// UpdateKind tells which fields of an Update are set.
type UpdateKind int

const (
	// StatusChanged sets Instance and Status.
	StatusChanged UpdateKind = iota + 1
	// HoverDone sets Req, Version, Outcome and, for Text, Content.
	HoverDone
	// CheckoutChanged sets Commit and Status, of the checkout that serves
	// Commit's documents: Starting while it is made, Ready once it is,
	// ServerFailed if it could not be.
	CheckoutChanged
)

// Update is news from the language servers.
type Update struct {
	Kind UpdateKind

	Instance lang.Instance
	Commit   change.ObjectID
	Status   ServerStatus

	Req     ReqID
	Version DocVersion
	Outcome Outcome
	// Content is the answer, for Text; its code comes coloured where the
	// viewer can colour it.
	Content Content
}
