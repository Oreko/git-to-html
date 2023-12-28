# How to Compile
## Using Go
run the following command
```
go build
```
## Using Nix
```
nix build
```
# How to Run
## After Building
If you built using nix, the binary will be at `results/bin/`
```
go-to-html path/to/repository
```
## Using Nix
```
nix run . -- path/to/repository
```

# What Gets Generated?
Inside the `public` directory we have the following:
1. `ref.html` --- This is the entry point for the repository and will display tags and branches
2. `c` --- This folder contains html files corresponding to each commit with the commit hash as the file name
3. `{branch_name}` --- A folder for each branch in your repository is additionally made.
   `t` --- This will contain the tree representation of your repository including seprate html for each folder and file.
## Styles
Currently, the generated html looks for a `static/style.css` one folder above the root (one folder above `public`). This will change soon to be specified by the caller.
