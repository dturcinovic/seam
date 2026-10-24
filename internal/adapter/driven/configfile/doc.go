// Package configfile reads seam's TOML configuration file into the table of
// languages and servers. It is the only package that reads the file. It
// finds no path, starts no server and checks no program for being
// installed; the rules of the table are lang's.
package configfile
