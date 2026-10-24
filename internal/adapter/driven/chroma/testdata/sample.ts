// A fixture.
interface Point { x: number }

export function hello(p: Point): string {
  const n: number = 42;
  return "hi" + p.x + n;
}
