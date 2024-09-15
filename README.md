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
git-to-html [-l log_length_limit] [-s relative/path/to/styles] path/to/repository "repository name goes here"
```
## Using Nix
Run the following command
```
nix run . -- [-l log_length_limit] [-s relative/path/to/styles] path/to/repository "repository name goes here"
```

## Efficiency Concerns
Calling Stat is quite expensive and is currently done on all commits both when generating the branch log and the html for each commit.
However, if there already exists a populated public (this is not the first run), the old commits will not be rewritten and therefore Stat won't be called.
We can also use the -l flag to specify the maximum number of times we run Stat when generating the log (which will be written whenever there exists a fresh commit).

The other bottleneck is that since we no longer call Stat for each commit when generating the html for each file in your repository, we can't easily determine if a file is fresh or not.
This means that we have to write the html for each file. What this all means is that repositories with lots of commits and branches will be slow to generate the first time, but much faster
on each rerun and will benefit from the -l flag. Similarly, repositories with a lot of files will be slow to generate in both initial and successive runs.

# What Gets Generated?
Inside the `public` directory we have the following:
1. `ref.html` --- This is the entry point for the repository and will display tags and branches
2. `c` --- This folder contains html files corresponding to each commit with the commit hash as the file name
3. `{branch_name}` --- A folder for each branch in your repository is additionally made.
4. `{branch_name}/t` --- This will contain the tree representation of your repository including seperate html for each folder and file.
## Styles
If you use the default configuration (e.g., don't pass -s), the generated html looks for a `static/style.css` one folder above the root (one folder above `public`).

# Todos
1. Allowing two compiling styles, one with the templates contained in the binary and one with the templates external.
2. Better representation of commit diffs for merges
