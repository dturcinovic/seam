// A fixture.
import { readFile } from "node:fs";

export function hello(name) {
  const n = 42;
  return `hi ${name}` + n;
}
