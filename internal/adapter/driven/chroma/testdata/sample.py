# A fixture.
import os

@dataclass
class Point:
    """A point."""
    x: int = 42

def hello(name):
    return f"hi {name}" + str(len(name))
