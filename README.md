# horse
```
            .''   gallop around your filesystem
  ._.-.___.' (`\ 
 //(        ( `'
'/ )\ ).__. ) 
' <' `\ ._/'\   
   `   \     \
```

## how to use

```bash
git clone http://github.com/if-not-nil/horse
cd horse
go build
cp ./horse ~/.local/bin/horse # or wherever your path points

# if you haven't tamed the horse yet, do this to see what the consequences of your actions could be
alias h="horse && cat /tmp/horselast"

# when you're comfortable,
alias h="horse && source /tmp/horselast"
```

- use it like you would `cd ls cd ls`  
- try opening folders and files with tab
- try pressing backspace when in a folder  
- try pressing enter
- try going to a folder and pressing enter  
- try doing `Down`, `Up`, `<C-n>`, `<C-p>`, `<C-j>`, `<C-k>`
- try doing `<C-c>` and see how its different from `Enter`

