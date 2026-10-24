// Package configfile reads seam's TOML configuration file: the table of
// languages and servers, the viewer's settings and the limits. It is the
// only package that reads the file. It finds no path, starts no server and
// checks no program for being installed; the rules of what it reads are
// those of lang, ui and limits.
package configfile
