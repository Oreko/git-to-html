# How to Compile
## Using Go
Run the following command
```
go build
```
## Using Nix
Run the following command
```
nix build
```
# How to Run
## After Building
If you built using nix, the binary will be at `results/bin/`. If you built using go, the binary will be in the root directory.

Run the following command
```
go-to-html path/to/repository "repository name goes here"
```
## Using Nix
Run the following command
```
nix run . -- path/to/repository "repository name goes here"
```

# What Gets Generated?
Inside the `public` directory we have the following:
1. `ref.html` --- This is the entry point for the repository and will display tags and branches
2. `c` --- This folder contains html files corresponding to each commit with the commit hash as the file name
3. `{branch_name}` --- A folder for each branch in your repository is additionally made.
4. `{branch_name}/t` --- This will contain the tree representation of your repository including seperate html for each folder and file.
## Styles
Currently, the generated html looks for a `static/style.css` one folder above the root (one folder above `public`). This will change soon to be specified by the caller.

# Todos
1. Better styling
2. Allowing two compiling styles, one with the templates contained in the binary and one with the templates external.
3. Not writing over an existing file if the file's timestamp is newer than its most recent commit's timestamp.
4. Better representation of commit diffs for merges
5. Handle commit notes
