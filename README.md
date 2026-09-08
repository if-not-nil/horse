# `horse, a file manager

<img width="1279" height="558" alt="image" src="https://github.com/user-attachments/assets/af82c907-1e1f-48c1-90df-8121f65216e0" />

**`how to use**

```bash
go install github.com/if-not-nil/horse@latest
# if you haven't tamed the horse yet, do this to see what the consequences of your actions could be
horse
# when you're comfortable,
alias h='eval "$(horse)"'
```

- use it like you would `cd ls cd ls`  
- try opening folders and files with tab
- try pressing backspace when in a folder  
- try pressing enter
- try going to a folder and pressing enter  
- try doing `Down`, `Up`, `<C-n>`, `<C-p>`, `<C-j>`, `<C-k>`
- try doing `<C-c>` and see how its different from `Enter`
- try selecting an image, it previews inline if your terminal speaks the kitty graphics protocol (kitty, ghostty, wezterm) or sixel (foot, xterm, mlterm, ...); set HORSE_SIXEL=1/0 to force sixel on/off

**`keymap**

```
Escape, <C-C>:
    exit without saving
Down, <C-J>, <C-N>:
    cursor down
Up, <C-K>, <C-P>:
    cursor up
C-w:
    delete word
C-e:
    go to `~` or `/`
C-a:
    bring up a prompt for creating files/directories (try qwer/asdf/zx)
C-s:
    copy the selected item's path
C-r:
    rename the selected item inline. type a path (a/b/c) to move it
    (Enter to confirm, Escape to cancel)
C-y:
    copy the selected file/dir. the destination is edited on a line below
    the source; type a path (a/b/c) to copy elsewhere (Enter/Escape)
C-x:
    selection mode: marks the current file. Tab marks/unmarks more,
    C-x again runs a bash command on them (% = the files, e.g. `cp % ./`;
    if there's no %, they're appended). Escape cancels
C-h, C-b:
    go up a directory
Tab, C-l, C-f:
    select an entry. if a file, open, if a directory, enter (go down)
Enter:
    cd to current/selected directory
Backspace:
    erase a character or go back a directory
~:
    jump back to the last directory you were in (toggles)
```

**`flags**
```
  -p	alias for -preview (default true)
  -preview
    	show a file preview on the right side (default true)
```

**`config**

rebind keys in `~/.config/horse/config` (`%APPDATA%\horse\config` on windows, or
`$XDG_CONFIG_HOME/horse/config`, or `$HORSE_CONFIG` to point anywhere).
one `action = key, key, ...` per line; `#` comments and blanks are ignored. a line replaces
that action's default keys. keys: `ctrl+x`, `tab`, `enter`, `esc`, `backspace`, `space`,
`up`/`down`/`left`/`right`, or a single character (e.g. `~`).

```
# example
down        = ctrl+j, ctrl+n, down
up          = ctrl+k, ctrl+p, up
updir       = ctrl+h, ctrl+b
select      = tab, ctrl+l, ctrl+f
cd          = enter
quit        = esc, ctrl+c
copypath    = ctrl+s
open        = ctrl+o
delete      = ctrl+d
rename      = ctrl+r
copy        = ctrl+y
multiselect = ctrl+x
create      = ctrl+a
delchar     = backspace
delword     = ctrl+w
home        = ctrl+e
prevdir     = ~
```

**`todo**
[ ] shared configurable read-line component for all inputs
[ ] consolidate more ops into C-x mode
