// Package sample is a fixture.
package sample

import "fmt"

/* A block comment
   over three lines
   ends here. */

type Point struct{ X int64 }

func Hello(name string) error {
	const answer = 42
	fmt.Println("hi\n", name, len(name), nil)
	return nil
}
