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
- try selecting an image, it previews inline if your terminal speaks the kitty graphics protocol (kitty, ghostty, wezterm)

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
    rename the selected item inline
    type a path (a/b/c) to move it
C-y:
    copy the selected file/dir
C-x:
    start selection, mark current file

    when selecting, run a bash command on the selection
    in it, the % placeholder will be substituted for the selection like {file1,file2}
C-o[pen]:
    open the selected file with the default application
C-d[elete]:
    delete the selected file or directory
C-v[isibility]:
    toggle the visibility of hidden files
Left, C-h, C-b:
    go up a directory
Tab, Right, C-l, C-f:
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
  -p[review]=true
     show a file preview on the right side (default true)
```

**`todo**
[ ] shared configurable read-line component for all inputs
[ ] consolidate more ops into C-x mode
