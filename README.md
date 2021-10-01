# google-docs-converter [![GoDoc](https://godoc.org/github.com/carlmjohnson/google-docs-converter?status.svg)](https://godoc.org/github.com/carlmjohnson/google-docs-converter) [![Go Report Card](https://goreportcard.com/badge/github.com/carlmjohnson/google-docs-converter)](https://goreportcard.com/report/github.com/carlmjohnson/google-docs-converter)

$DESCRIPTION

## Installation

First install [Go](http://golang.org).

If you just want to install the binary to your current directory and don't care about the source code, run

```bash
GOBIN=$(pwd) GOPATH=$(mktemp -d) go install github.com/carlmjohnson/google-docs-converter/cmd/gdocs@latest
```

## Screenshots

```bash
$ gdocs -h
gdocs - extracts a document from Google Docs

Usage:

        gdocs [options]

Options:
  -id string
        ID for Google Doc
  -read-doc path
        path to read document from instead of Google Docs
  -silent
        don't log debug output
  -write-doc path
        path to write out document
```
