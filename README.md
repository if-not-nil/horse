# `horse, a filepicker

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

**`plans**

- [x] (C-h or C-b), (C-l or C-f) is up and down direcectories
- [x] C-s copies the selected item's path
- [x] retain the last selection when going between directories
- [ ] use kitty image protocol for the preview window
- [ ] C-r to rename file (like netrw/dired, with ability to move). it will start editing inline
- [ ] C-S-r to copy file, puts a line below the line where you edit the path
- [ ] C-x to enter selection mode, where tab adds to the selection
    file under cursor is selected by default, so that you can quickly doubletap C-x C-x
  - [ ] C-x again to open a command prompt for putting that into bash (`cp {f1.txt,f2.rv} ./`)
        this will be the only way to batch-do anything with files
- [x] ~ to move back-and-forth between the last two directories (like vim's C-6)
