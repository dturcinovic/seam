// Package lang names the languages of a session and the servers that hover
// over them: which language a file or a code fence is written in, which
// server and languageId it has, and which workspace a file belongs to. It
// reads no file and no configuration: it judges a path and the start of the
// text it is given, and asks its caller whether a root marker exists.
package lang
