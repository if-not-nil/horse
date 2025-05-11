# horse
> gallops around your filesystem

## how to use
use it like you would `cd ls cd ls`  
try pressing tab for going places and autocompletion  
try pressing backspace when in a folder  
try opening a file  
try going to a folder and pressing enter  

## installation

```bash
pipx install git+https://github.com/if-not-nil/horse
```
make sure your $EDITOR variable is set

then, you'll have to integrate it into your shell
add this to the end of your shell's config

### bash and zsh:
```bash
function h() {
    horse && source ~/.config/.horselast
}
```
### fish:
```fish
function h
    horse; and source ~/.config/.horselast ^/dev/null
end
```  
