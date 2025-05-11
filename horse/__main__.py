import curses

from . import app


def main() -> None:
    try:
        result = curses.wrapper(app.main)
        if result:
            from pathlib import Path

            tempfile = Path.home() / ".config" / ".horselast"
            tempfile.write_text(result)
    except KeyboardInterrupt:
        import sys

        sys.exit(1)
