package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/carlmjohnson/flagext"
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
	fl.StringVar(&app.outputDoc, "write-doc", "", "`path` to write out document")
	fl.StringVar(&app.inputDoc, "read-doc", "", "`path` to read document from instead of Google Docs")

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
	if err := fl.Parse(args); err != nil {
		return err
	}
	if err := flagext.ParseEnv(fl, AppName); err != nil {
		return err
	}

	app.docid = normalizeID(app.docid)
	return nil
}

type appEnv struct {
	docid     string
	inputDoc  string
	outputDoc string
	*log.Logger
}

func (app *appEnv) Exec() (err error) {
	app.Println("starting Google Docs service")
	ctx := context.Background()
	var doc *docs.Document
	if app.inputDoc == "" {
		srv, err := docs.NewService(ctx)
		if err != nil {
			return err
		}

		app.Printf("getting %q", app.docid)
		doc, err = srv.Documents.Get(app.docid).Do()
		if err != nil {
			return err
		}
		if app.outputDoc != "" {
			b, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				return err
			}

			if err = os.WriteFile(app.outputDoc, b, 0644); err != nil {
				return err
			}
		}
	} else {
		app.Printf("reading %q", app.inputDoc)
		b, err := os.ReadFile(app.inputDoc)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, doc); err != nil {
			return err
		}
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
		Type: html.DocumentNode,
	}
	listInfo := buildListInfo(doc.Lists)
	objectInfo := buildObjectInfo(doc.InlineObjects)
	for _, el := range doc.Body.Content {
		convertEl(n, el, listInfo, objectInfo)
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

func buildListInfo(lists map[string]docs.List) map[string]string {
	m := map[string]string{}
	for id, list := range lists {
		if list.ListProperties == nil {
			continue
		}
		listType := "ul"
		if list.ListProperties.NestingLevels[0].GlyphType != "" {
			listType = "ol"
		}
		m[id] = listType
	}
	return m
}

func buildObjectInfo(objs map[string]docs.InlineObject) map[string][]string {
	m := map[string][]string{}
	for id, obj := range objs {
		if obj.InlineObjectProperties == nil {
			continue
		}
		innerObj := obj.InlineObjectProperties.EmbeddedObject
		src := ""
		if innerObj.ImageProperties != nil {
			src = innerObj.ImageProperties.ContentUri
		}
		m[id] = []string{
			"src", src,
			"title", innerObj.Title,
			"alt", innerObj.Description,
		}
	}
	return m
}

func convertEl(n *html.Node, el *docs.StructuralElement, listInfo map[string]string, objInfo map[string][]string) {
	if el.Table != nil && el.Table.TableRows != nil {
		table := newElement("table")
		n.AppendChild(table)
		for _, row := range el.Table.TableRows {
			rowEl := newElement("tr")
			table.AppendChild(rowEl)
			if row.TableCells != nil {
				for _, cell := range row.TableCells {
					cellEl := newElement("td")
					rowEl.AppendChild(cellEl)
					for _, content := range cell.Content {
						convertEl(cellEl, content, listInfo, objInfo)
					}
				}
			}
		}
	}
	if el.Paragraph == nil {
		return
	}
	if el.Paragraph.Bullet != nil {
		listType := listInfo[el.Paragraph.Bullet.ListId]
		ul := lastChildOrNewElement(n, listType)
		li := newElement("li")
		ul.AppendChild(li)
		n = li
	}

	blockType := tagForNamedStyle[el.Paragraph.ParagraphStyle.NamedStyleType]

	n.AppendChild(newElement(blockType))

	for _, subel := range el.Paragraph.Elements {
		if subel.HorizontalRule != nil {
			n.AppendChild(newElement("hr"))
		}

		if subel.InlineObjectElement != nil {
			inner := lastChildOrNewElement(n, blockType)
			attrs := objInfo[subel.InlineObjectElement.InlineObjectId]
			inner.AppendChild(newElement("img", attrs...))
		}

		if subel.TextRun == nil {
			continue
		}

		if strings.TrimSpace(subel.TextRun.Content) == "" {
			continue
		}

		inner := lastChildOrNewElement(n, blockType)
		if subel.TextRun.TextStyle != nil {
			if len(subel.TextRun.SuggestedDeletionIds) > 0 {
				newinner := newElement("del")
				inner.AppendChild(newinner)
				inner = newinner
			}
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

func lastChildOrNewElement(p *html.Node, tag string, attrs ...string) *html.Node {
	if p.LastChild != nil && p.LastChild.Data == tag {
		return p.LastChild
	}
	n := newElement(tag, attrs...)
	p.AppendChild(n)
	return n
}

func appendText(n *html.Node, text string) {
	n.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: text,
	})
}
