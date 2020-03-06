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
	"golang.org/x/net/html"
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

	app.Printf("getting %q", app.docid)
	doc, err := srv.Documents.Get(app.docid).Do()
	if err != nil {
		return err
	}

	app.Printf("got %q", doc.Title)

	n := convert(doc)

	return html.Render(os.Stdout, n)
}

func normalizeID(id string) string {
	id = strings.TrimPrefix(id, "https://docs.google.com/document/d/")
	if i := strings.LastIndexByte(id, '/'); i != -1 {
		id = id[:i]
	}
	return id
}

func convert(doc *docs.Document) (n *html.Node) {
	n = &html.Node{
		Type: html.ElementNode,
		Data: "div",
	}
	for _, el := range doc.Body.Content {
		convertEl(n, el)
	}
	return
}

var tagForNamedStyle = map[string]string{
	"NAMED_STYLE_TYPE_UNSPECIFIED": "div",
	"NORMAL_TEXT":                  "p",
	"TITLE":                        "h1",
	"SUBTITLE":                     "h1",
	"HEADING_1":                    "h1",
	"HEADING_2":                    "h2",
	"HEADING_3":                    "h3",
	"HEADING_4":                    "h4",
	"HEADING_5":                    "h5",
	"HEADING_6":                    "h6",
}

func convertEl(n *html.Node, el *docs.StructuralElement) {
	if el.Paragraph == nil {
		return
	}

	block := html.Node{
		Type: html.ElementNode,
		Data: tagForNamedStyle[el.Paragraph.ParagraphStyle.NamedStyleType],
	}
	for _, subel := range el.Paragraph.Elements {
		if subel.HorizontalRule != nil {
			n.AppendChild(newElement("hr"))
		}
		if subel.TextRun == nil {
			continue
		}
		inner := &block
		if subel.TextRun.TextStyle != nil {
			if subel.TextRun.TextStyle.Link != nil {
				newinner := newElement("a", "href", subel.TextRun.TextStyle.Link.Url)
				inner.AppendChild(newinner)
				inner = newinner
			}
			if subel.TextRun.TextStyle.Bold {
				newinner := newElement("strong")
				inner.AppendChild(newinner)
				inner = newinner
			}
			if subel.TextRun.TextStyle.Italic {
				newinner := newElement("em")
				inner.AppendChild(newinner)
				inner = newinner
			}
		}
		appendText(inner, subel.TextRun.Content)
	}
	n.AppendChild(&block)
}

func newElement(tag string, attrs ...string) *html.Node {
	var attrslice []html.Attribute
	if len(attrs) > 0 {
		if len(attrs)%2 != 0 {
			panic("uneven number of attr/value pairs")
		}
		attrslice = make([]html.Attribute, len(attrs)/2)
		for i := range attrslice {
			attrslice[i] = html.Attribute{
				Key: attrs[i*2],
				Val: attrs[i*2+1],
			}
		}
	}
	return &html.Node{
		Type: html.ElementNode,
		Data: tag,
		Attr: attrslice,
	}
}

func appendText(n *html.Node, text string) {
	n.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: text,
	})
}
