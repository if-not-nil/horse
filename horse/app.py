"""navigates around your file."""

import curses
import sys
import time
from pathlib import Path

from rapidfuzz import fuzz, process

ROWS = 20


def cd(state: dict, path: Path) -> None:
    target_path = state["cwd"] / path
    if str(path) == "..":
        target_path = state["cwd"].parent
    if target_path.is_dir():
        state["cwd"] = target_path
        state["lines"] = list(state["cwd"].glob("*"))
        update(state, "")


def update(state: dict, input_str: str) -> None:
    try:
        state["cursor"] = 0
        entries = [str(p.relative_to(state["cwd"])) for p in state["lines"]]
        if not input_str.strip():
            state["results"] = [(entry, 0) for entry in entries]
        else:
            matched = process.extract(
                input_str,
                entries,
                scorer=fuzz.WRatio,
                limit=ROWS,
            )
            state["results"] = (
                [(match[0], match[1]) for match in matched] if matched else []
            )
    except Exception:  # noqa: BLE001 i dont really care
        state["results"] = []


def quit_print(state: dict, stdscr: curses.window, input_str: str) -> str:
    stdscr.clear()
    stdscr.refresh()
    curses.endwin()
    results = state["results"]
    cursor = state["cursor"]
    cwd = state["cwd"]
    obj = cwd if not results or input_str == "" else cwd / results[cursor][0]
    return f"cd {obj.as_posix()}" if obj.is_dir() else f"$EDITOR {obj.as_posix()}"


def main(stdscr: curses.window) -> str:
    state = {
        "cwd": Path.cwd(),
        "lines": list(Path.cwd().glob("*")),
        "cursor": 0,
        "results": [],
        "max_rows": 0,
    }
    input_str = ""
    update(state, input_str)
    init_curses(stdscr)

    while True:
        max_y, _ = stdscr.getmaxyx()
        state["max_rows"] = max_y - 3

        key = stdscr.getch()
        if key != -1:
            input_str, exit_requested = handle_key(state, key, input_str)
            if exit_requested:
                return quit_print(state, stdscr, input_str)

        draw_screen(state, stdscr, input_str)
        time.sleep(0.05)


def init_curses(stdscr: curses.window) -> None:
    curses.start_color()
    curses.init_pair(1, curses.COLOR_WHITE, curses.COLOR_BLACK)  # normal
    curses.init_pair(2, 8, curses.COLOR_BLACK)  # suggestion
    curses.init_pair(3, curses.COLOR_CYAN, curses.COLOR_BLACK)  # directory
    curses.set_escdelay(1)
    stdscr.timeout(100)


def handle_key(
    state: dict,
    key: int,
    input_str: str,
) -> tuple[str, bool]:
    cursor = state["cursor"]
    results = state["results"]
    cwd = state["cwd"]

    match key:
        case 9:  # Tab
            if not results:
                return input_str, True
            line = Path(results[cursor][0])
            input_str = ""
            full_path = cwd / line
            if full_path.is_dir():
                cd(state, line)
            else:
                return input_str, True

        case 10:  # Enter
            return input_str, True

        case 27:  # Escape
            sys.exit(1)

        case 263:  # Backspace
            if input_str == "":
                cd(state, Path(".."))
            else:
                input_str = input_str[:-1]
            update(state, input_str)

        case 258:  # Down
            if results:
                state["cursor"] = (cursor + 1) % len(results)

        case 259:  # Up
            if results:
                state["cursor"] = (cursor - 1) % len(results)

        case _:  # Character input
            try:
                char = chr(key)
                char.encode("ascii")
                input_str += char
                update(state, input_str)
            except UnicodeEncodeError:
                pass

    return input_str, False


def get_autocomplete(state: dict, input_str: str) -> str:
    results = state["results"]
    if results:
        suggestion = str(results[0][0])
        if suggestion.startswith(input_str):
            return suggestion[len(input_str) :]
    return ""


def draw_screen(state: dict, stdscr: curses.window, input_str: str) -> None:
    stdscr.clear()

    cwd = state["cwd"]
    cursor = state["cursor"]
    results = state["results"]
    max_rows = state["max_rows"]

    actual_len = len(str(cwd)) + 1 + len(input_str)
    comp = get_autocomplete(state, input_str)

    stdscr.addstr(0, 0, f"{cwd}/{input_str}", curses.color_pair(1))
    stdscr.addstr(0, actual_len, comp, curses.color_pair(2) | curses.A_DIM)

    visible_results = results[:max_rows]
    for i, (line, _) in enumerate(visible_results):
        full_path = cwd / line
        display_name = f"{line}/" if full_path.is_dir() else line
        style = curses.color_pair(1) if full_path.is_file() else curses.color_pair(3)
        if cursor == i:
            style = style | curses.A_REVERSE
        stdscr.addstr(2 + i, 0, display_name, style)

    stdscr.move(0, actual_len)
    stdscr.refresh()


if __name__ == "__main__":
    try:
        result = curses.wrapper(main)
        if result:
            tempfile = Path.home() / ".config" / ".horselast"
            tempfile.write_text(result)
    except KeyboardInterrupt:
        sys.exit(1)
