# horse

<img width="367" height="190" alt="image" src="https://github.com/user-attachments/assets/de70b9d1-6810-4834-8eb2-68369b8c5427" />

```
            .''   generic filepicker with a focus on ergonomics, inspired by zellij's
  ._.-.___.' (`\ 
 //(        ( `'
'/ )\ ).__. ) 
' <' `\ ._/'\   
   `   \     \
```


** how to use **

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

** keymap **

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
Tab:
    select an entry. if a file, open, if a directory, enter
Enter:
    cd to current directory
Backspace:
    erase a character or go back a directory
```

** flags **
```
  -p	alias for -preview (default true)
  -preview
    	show a file preview on the right side (default true)
```

**plans**

- [ ] (C-h or C-b), (C-l or C-f) is up and down direcectories
- [ ] C-s copies the selected item's path
- [ ] retain the last selection when going between directories
