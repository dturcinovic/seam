package change

// Head is what HEAD stands on: a branch, by its name without refs/heads/,
// or, detached, a commit. During a rebase it is the branch being rebased,
// though HEAD is detached meanwhile. The zero value is not known.
type Head struct {
	Branch string
	Commit ObjectID
}

// IsZero reports whether h names neither branch nor commit.
func (h Head) IsZero() bool { return h.Branch == "" && h.Commit.IsZero() }

// String names h as the user would: the branch, or the commit.
func (h Head) String() string {
	if h.Branch != "" {
		return h.Branch
	}
	return h.Commit.String()
}
