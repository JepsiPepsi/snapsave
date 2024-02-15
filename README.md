## SnapChat Public profile scraper

## Very simple SnapChat scraper for public profiles built with Go
#### - Downloads stories, highlights, spotlights and curated highlights
#### - Name files by ID, and checks for existing files before  trying to download, aka no overwrites
#### - Can run on a given interval (minutes), for as many users as you want
#### - Bugs? Feature request? Create an issue
#### - Easily build small and portable executables using Go
---
### Usage

| Flag       | Behaviour                                                               | Default   | Type   |
|------------|-------------------------------------------------------------------------|-----------|--------|
| --interval | Sets run interval in minutes                                            | 0         | int    |
| --userfile | Specify path to a .txt file to read username from (Seperated by newline | users.txt | string |
| --output   | Specify output directory                                                | ./        | string |
| [username] | Scrapes data for a single user                                          | -         | -      |


#### I have provided a release section with prebuilt binaries, but I would highly suggest you build binaries yourself whenever you can, as a safety precaution. (Takes 5 minutes) Read below.
---
### Build from source

#### Install go
```bash 
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
```

#### Remove previous install, and extract downloaded tarball to /usr/local
```bash
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
```
#### Add Go binary to PATH, and set GOPATH to home directory. Add the following to ~/.bashrc
```bash
export PATH=$PATH:/usr/local/go/bin
export GOPATH="$HOME/go"
```
#### Clone repo, cd in to it, and run main.go

```bash
git clone git@github.com:JepsiPepsi/snapsave.git &&
cd value-chain-monitor &&
go run main.go
```
#### Build program

```bash
go mod donwload &&
Windows: GOOS=windows GOARCH=amd64 cgo_enabled=0 go build -o snapsave.exe main.go
Linux: GOOS=linux GOARCH=amd64 cgo_enabled=0 go build -o snapsave main.go
```
