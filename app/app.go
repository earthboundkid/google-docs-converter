package app

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/carlmjohnson/flagext"
	"github.com/peterbourgon/ff"
	"google.golang.org/api/docs/v1"
)

const AppName = "gdocs"

func CLI(args []string) error {
	var app appEnv
	err := app.ParseArgs(args)
	if err == nil {
		err = app.Exec()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	return err
}

func (app *appEnv) ParseArgs(args []string) error {
	fl := flag.NewFlagSet(AppName, flag.ContinueOnError)
	fl.StringVar(&app.docid, "id", "", "ID for Google Doc")
	app.Logger = log.New(nil, AppName+" ", log.LstdFlags)
	fl.Var(
		flagext.Logger(app.Logger, flagext.LogSilent),
		"silent",
		`don't log debug output`,
	)

	fl.Usage = func() {
		fmt.Fprintf(fl.Output(), `gdocs - extracts a document from Google Docs

Usage:

	gdocs [options]

Options:
`)
		fl.PrintDefaults()
		fmt.Fprintln(fl.Output(), "")
	}
	if err := ff.Parse(fl, args, ff.WithEnvVarPrefix("GO_CLI")); err != nil {
		return err
	}

	app.docid = normalizeID(app.docid)
	return nil
}

type appEnv struct {
	docid string
	*log.Logger
}

func (app *appEnv) Exec() (err error) {
	app.Println("starting Google Docs service")
	ctx := context.Background()
	srv, err := docs.NewService(ctx)
	if err != nil {
		return err
	}

	app.Printf("getting %s", app.docid)
	doc, err := srv.Documents.Get(app.docid).Do()
	if err != nil {
		return err
	}

	app.Printf("got doc %v", doc)

	return err
}

func normalizeID(id string) string {
	id = strings.TrimPrefix(id, "https://docs.google.com/document/d/")
	if i := strings.LastIndexByte(id, '/'); i != -1 {
		id = id[:i]
	}
	return id
}
