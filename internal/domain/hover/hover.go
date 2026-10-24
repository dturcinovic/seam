package hover

import "seam/internal/domain/lang"

// DocVersion counts the texts a document has had in a session. It rises
// exactly when the text changes, so an answer to an older version can be
// told apart from one to the text on screen.
type DocVersion uint64

// Document is the text a language server is to see for a file: the right
// side of the diff on screen. Path is relative to the repository root in
// slash form and names the document.
type Document struct {
	Path    string
	Lang    lang.Lang
	Version DocVersion
	Text    string
}

// ReqID names a hover request. The viewer hands out strictly increasing
// ones before it asks, so an answer finds its request whenever it comes.
type ReqID uint64

// Position is where a hover asks: in the document at Path as of Version,
// at a byte offset into a line, both counted from zero.
type Position struct {
	Req     ReqID
	Path    string
	Version DocVersion
	Line    int
	Byte    int
}

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
	// HoverDone sets Req, Version, Outcome and, for Text, Text.
	HoverDone
)

// Update is news from the language servers.
type Update struct {
	Kind UpdateKind

	Instance lang.Instance
	Status   ServerStatus

	Req     ReqID
	Version DocVersion
	Outcome Outcome
	// Text is the answer as Markdown.
	Text string
}
